package dragontoothmg

import (
	"math/bits"
	"slices"
	"strings"
)

// Each bitboard shall use little-endian rank-file mapping:
// 56  57  58  59  60  61  62  63
// 48  49  50  51  52  53  54  55
// 40  41  42  43  44  45  46  47
// 32  33  34  35  36  37  38  39
// 24  25  26  27  28  29  30  31
// 16  17  18  19  20  21  22  23
// 8   9   10  11  12  13  14  15
// 0   1   2   3   4   5   6   7
// The binary bitboard uint64 thus uses this ordering:
// MSB---------------------------------------------------LSB
// H8 G8 F8 E8 D8 C8 B8 A8 H7 ... A2 H1 G1 F1 E1 D1 C1 B1 A1

// The board type, which uses little-endian rank-file mapping.
type Board struct {
	Wtomove       bool
	enpassant     uint8 // square id (16-23 or 40-47) where en passant capture is possible
	castlerights  uint8
	Halfmoveclock uint8
	Fullmoveno    uint16
	White         Bitboards
	Black         Bitboards
	hash          uint64

	// Contains main line of the game, with additional
	History     []History
	termination Termination
}

type Termination uint16

const (
	TerminationNone                 = 0
	TerminationCheckmate            = 1
	TerminationStalemate            = 2
	TerminationFiftyMovesRule       = 4
	TerminationInsufficientMaterial = 8
	TerminationRepetition           = 16
)

func (t Termination) String() string {

	if t == TerminationNone {
		return "TerminationNone"
	}

	termination := strings.Builder{}
	if t&TerminationCheckmate != 0 {
		termination.WriteString("TerminationCheckmate|")
	}
	if t&TerminationStalemate != 0 {
		termination.WriteString("TerminationStalemate|")
	}
	if t&TerminationFiftyMovesRule != 0 {
		termination.WriteString("TerminationFiftyMovesRule|")
	}
	if t&TerminationInsufficientMaterial != 0 {
		termination.WriteString("TerminationInsufficientMaterial|")
	}

	s := termination.String()
	return s[:len(s)-1]
}

// Internal structure to store history for undoing moves and detecting repetitions
type History struct {
	// Stores the hash before making the move with Make() (so that Undo() can restore it)
	hashBefore uint64
	// Stores the hash after making the move with Make() (so that IsRepetition can work)
	hashCurrent uint64

	// fields captured by original closure, many are probably redundant
	resetHalfmoveClockFrom                                                   int     // required
	oldRookLoc, newRookLoc                                                   uint8   // not req
	flippedKsCastle, flippedQsCastle, flippedOppKsCastle, flippedOppQsCastle bool    // not req
	capturedBitboard                                                         *uint64 // required but may be converted to uint8 square
	Move                                                                     Move    // required
	oldEpCaptureSquare                                                       uint8   // not req
	castleStatus                                                             int
	capturedPieceType                                                        Piece // required
}

// Create a new board in the starting position.
func NewBoard() *Board {
	b := ParseFen(Startpos)
	return &b
}

// Return the Zobrist hash value for the board.
// The hash value does NOT change with the turn number, nor the draw move counter.
// All other elements of the Board type affect the hash.
// This function is cheap to call, since the hash is incrementally updated.
func (b *Board) Hash() uint64 {
	//b.hash = recomputeBoardHash(b)
	return b.hash
}

// Returns true if the given move is legal in the current position.
func (b *Board) IsLegal(m Move) bool {
	return slices.Contains(b.GenerateLegalMoves(), m)
}

// Returns pre-calculated termination reason, use 'IsTerminated' to
// calculate it
func (b *Board) Termination() Termination {
	return b.termination
}

// Calculates whether the game is terminated by any of the rules:
//
// - Checkmate
//
// - Stalemate
//
// - Fifty-move rule
//
// - Threefold repetition
//
// - Insufficient material
//
// The parameter 'moveCount' is the number of legal moves in the current position,
// which can be obtained by calling 'GenerateLegalMoves()' and taking the length of the result.
// To get a more verbose termination reason, call 'Termination()' after this function.
func (b *Board) IsTerminated(moveCount int) bool {
	if b.Halfmoveclock >= 100 {
		b.termination |= TerminationFiftyMovesRule
	}

	if moveCount == 0 {
		if b.OurKingInCheck() {
			b.termination |= TerminationCheckmate
		} else {
			b.termination |= TerminationStalemate
		}
	}

	return b.termination != TerminationNone || b.IsRepetition(3) || b.IsInsufficientMaterial()
}

// Returns true if the current position has occurred 'nTimes' times (or more) in the game history
func (b *Board) IsRepetition(nTimes int) bool {
	count := 0
	h := b.Hash()
	for i := len(b.History) - 1; i >= 0 && count < 3; i -= 2 {
		if b.History[i].hashCurrent == h {
			count++
		}
	}

	if count >= nTimes {
		b.termination |= TerminationRepetition
		return true
	}

	return false
}

// Source https://www.chessprogramming.org/Material#InsufficientMaterial
// According to FIDE: KB vs K is a draw, as is KN vs K and KNN vs K
func (b *Board) IsInsufficientMaterial() bool {

	// If there are still rooks, queens or pawns on the board, the
	// game isn't terminated yet
	if (b.White.Queens|b.White.Rooks|b.White.Pawns) != 0 ||
		(b.Black.Queens|b.Black.Rooks|b.Black.Pawns) != 0 {
		return false
	}

	// King vs king
	if b.White.All == b.White.Kings && b.Black.All == b.Black.Kings {
		b.termination |= TerminationInsufficientMaterial
		return true
	}

	// King and bishop vs king
	if (b.White.All == (b.White.Kings|b.White.Bishops) && b.Black.All == b.Black.Kings) ||
		(b.Black.All == (b.Black.Kings|b.Black.Bishops) && b.White.All == b.White.Kings) {
		b.termination |= TerminationInsufficientMaterial
		return true
	}

	// King and knight vs king
	if (b.White.All == (b.White.Kings|b.White.Knights) && b.Black.All == b.Black.Kings) ||
		(b.Black.All == (b.Black.Kings|b.Black.Knights) && b.White.All == b.White.Kings) {
		b.termination |= TerminationInsufficientMaterial
		return true
	}

	// Only 1 bishop each, and they are on the same color
	if b.White.Bishops != 0 && b.Black.Bishops != 0 &&
		(b.White.Bishops&(b.White.Bishops-1)) == 0 && // only 1 bishop
		(b.Black.Bishops&(b.Black.Bishops-1)) == 0 &&
		((b.White.All == (b.White.Kings | b.White.Bishops)) &&
			(b.Black.All == (b.Black.Kings | b.Black.Bishops))) {
		// Get the coordinates of the bishops
		wBishopSquare := Square(bits.TrailingZeros64(b.White.Bishops))
		bBishopSquare := Square(bits.TrailingZeros64(b.Black.Bishops))

		// Check if they are on the same color
		if (wBishopSquare.Rank()+wBishopSquare.File())%2 == (bBishopSquare.Rank()+bBishopSquare.File())%2 {
			b.termination |= TerminationInsufficientMaterial
			return true
		}
	}

	return false
}

// Returns a deep copy of the board, including its history.
func (b Board) Clone() *Board {
	history := make([]History, len(b.History))
	copy(history, b.History)
	return &Board{
		Wtomove:       b.Wtomove,
		enpassant:     b.enpassant,
		castlerights:  b.castlerights,
		Halfmoveclock: b.Halfmoveclock,
		Fullmoveno:    b.Fullmoveno,
		White:         b.White,
		Black:         b.Black,
		hash:          b.hash,

		// Added
		History:     history,
		termination: b.termination,
	}
}

// Castle rights helpers. Data stored inside, from LSB:
// 1 bit: White castle queenside
// 1 bit: White castle kingside
// 1 bit: Black castle queenside
// 1 bit: Black castle kingside
// This just indicates whether castling rights have been lost, not whether
// castling is actually possible.

// Castling helper functions for all 16 possible scenarios
func (b *Board) WhiteCanCastleQueenside() bool {
	return b.castlerights&1 == 1
}
func (b *Board) WhiteCanCastleKingside() bool {
	return (b.castlerights&0x2)>>1 == 1
}
func (b *Board) BlackCanCastleQueenside() bool {
	return (b.castlerights&0x4)>>2 == 1
}
func (b *Board) BlackCanCastleKingside() bool {
	return (b.castlerights&0x8)>>3 == 1
}
func (b *Board) CanCastleQueenside() bool {
	if b.Wtomove {
		return b.WhiteCanCastleQueenside()
	} else {
		return b.BlackCanCastleQueenside()
	}
}
func (b *Board) CanCastleKingside() bool {
	if b.Wtomove {
		return b.WhiteCanCastleKingside()
	} else {
		return b.BlackCanCastleKingside()
	}
}
func (b *Board) OppCanCastleQueenside() bool {
	if b.Wtomove {
		return b.BlackCanCastleQueenside()
	} else {
		return b.WhiteCanCastleQueenside()
	}
}
func (b *Board) OppCanCastleKingside() bool {
	if b.Wtomove {
		return b.BlackCanCastleKingside()
	} else {
		return b.WhiteCanCastleKingside()
	}
}
func (b *Board) flipWhiteQueensideCastle() {
	b.castlerights = b.castlerights ^ (1)
	b.hash ^= castleRightsZobristC[1]
}
func (b *Board) flipWhiteKingsideCastle() {
	b.castlerights = b.castlerights ^ (1 << 1)
	b.hash ^= castleRightsZobristC[0]
}
func (b *Board) flipBlackQueensideCastle() {
	b.castlerights = b.castlerights ^ (1 << 2)
	b.hash ^= castleRightsZobristC[3]
}
func (b *Board) flipBlackKingsideCastle() {
	b.castlerights = b.castlerights ^ (1 << 3)
	b.hash ^= castleRightsZobristC[2]
}
func (b *Board) flipQueensideCastle() {
	if b.Wtomove {
		b.flipWhiteQueensideCastle()
	} else {
		b.flipBlackQueensideCastle()
	}
}
func (b *Board) flipKingsideCastle() {
	if b.Wtomove {
		b.flipWhiteKingsideCastle()
	} else {
		b.flipBlackKingsideCastle()
	}
}
func (b *Board) flipOppQueensideCastle() {
	if b.Wtomove {
		b.flipBlackQueensideCastle()
	} else {
		b.flipWhiteQueensideCastle()
	}
}
func (b *Board) flipOppKingsideCastle() {
	if b.Wtomove {
		b.flipBlackKingsideCastle()
	} else {
		b.flipWhiteKingsideCastle()
	}
}

// Contains bitboard representations of all the pieces for a side.
type Bitboards struct {
	Pawns   uint64
	Bishops uint64
	Knights uint64
	Rooks   uint64
	Queens  uint64
	Kings   uint64
	All     uint64
}

// Data stored inside, from LSB
// 6 bits: destination square
// 6 bits: source square
// 3 bits: promotion

// Move bitwise structure; internal implementation is private.
type Move uint16

func (m *Move) To() uint8 {
	return uint8(*m & 0x3F)
}
func (m *Move) From() uint8 {
	return uint8((*m & 0xFC0) >> 6)
}

// Whether the move involves promoting a pawn.
func (m *Move) Promote() Piece {
	return Piece((*m & 0x7000) >> 12)
}
func (m *Move) Setto(s Square) *Move {
	*m = *m & ^(Move(0x3F)) | Move(s)
	return m
}
func (m *Move) Setfrom(s Square) *Move {
	*m = *m & ^(Move(0xFC0)) | (Move(s) << 6)
	return m
}
func (m *Move) Setpromote(p Piece) *Move {
	*m = *m & ^(Move(0x7000)) | (Move(p) << 12)
	return m
}
func (m *Move) String() string {
	/*return fmt.Sprintf("[from: %v, to: %v, promote: %v]",
	IndexToAlgebraic(Square(m.From())), IndexToAlgebraic(Square(m.To())), m.Promote())*/
	if *m == 0 {
		return "0000"
	}
	result := IndexToAlgebraic(Square(m.From())) + IndexToAlgebraic(Square(m.To()))
	switch m.Promote() {
	case Queen:
		result += "q"
	case Knight:
		result += "n"
	case Rook:
		result += "r"
	case Bishop:
		result += "b"
	default:
	}
	return result
}

// Square index values from 0-63.
type Square uint8

// Returns the rank (0-7) of the square.
func (s Square) Rank() uint8 {
	return uint8(s >> 3)
}

// Returns the file (0-7) of the square.
func (s Square) File() uint8 {
	return uint8(s & 7)
}

// Piece types; valid in range 0-6, as indicated by the constants for each piece.
type Piece uint8

const (
	Nothing = iota
	Pawn    = iota
	Knight  = iota // list before bishop for promotion loops
	Bishop  = iota
	Rook    = iota
	Queen   = iota
	King    = iota
)
