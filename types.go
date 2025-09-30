package dragontoothmg

import (
	"slices"
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

	// Added
	history         []History
	termination     Termination
	irreversibleIdx int
}

type Termination uint16

const (
	TerminationNone                 = 0
	TerminationCheckmate            = 1
	TerminationStalemate            = 2
	TerminationFiftyMovesRule       = 4
	TerminationInsufficientMaterial = 8
)

func (t Termination) String() string {
	switch t {
	case TerminationCheckmate:
		return "TerminationCheckmate"
	case TerminationStalemate:
		return "TerminationStalemate"
	case TerminationFiftyMovesRule:
		return "TerminationFiftyMovesRule"
	case TerminationInsufficientMaterial:
		return "TerminationInsufficientMaterial"
	default:
		return "TerminationNone"
	}
}

// Internal structure to store history for undoing moves and detecting repetitions
type History struct {
	// Stores the hash before making the move with Make() (so that Undo() can restore it)
	hashBefore uint64
	// Stores the hash after making the move with Make() (so that IsRepetition can work)
	hashCurrent uint64

	// fields captured by original closure, probably many are redundant
	resetHalfmoveClockFrom                                                   int
	oldRookLoc, newRookLoc                                                   uint8
	flippedKsCastle, flippedQsCastle, flippedOppKsCastle, flippedOppQsCastle bool
	capturedBitboard                                                         *uint64
	actuallyPerformedEpCapture                                               bool
	m                                                                        Move
	oldEpCaptureSquare                                                       uint8
	castleStatus                                                             int
	capturedPieceType, pieceType, promotedToPieceType                        Piece
	destTypeBitboard, pieceTypeBitboard                                      *uint64
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

func (b *Board) IsTerminated(moveCount int) bool {
	if b.Halfmoveclock >= 100 {
		b.termination = TerminationFiftyMovesRule
	}

	if moveCount == 0 {
		if b.OurKingInCheck() {
			b.termination = TerminationCheckmate
		} else {
			b.termination = TerminationStalemate
		}
	}

	return b.termination != TerminationNone || b.IsRepetition(3) || b.IsInsufficientMaterial()
}

func (b *Board) IsRepetition(nTimes int) bool {
	count := 0
	h := b.Hash()
	for i := len(b.history) - 1; i >= b.irreversibleIdx && count < 3; i -= 2 {
		if b.history[i].hashCurrent == h {
			count++
		}
	}

	return count == nTimes
}

func oneOrLess(b, k uint64) bool {
	return k == 0 || ((k & (k - 1)) == 0) || b == 0 || ((b & (b - 1)) == 0)
}

func (b *Board) IsInsufficientMaterial() bool {

	// If there are still rooks, queens or pawns on the board, the
	// game isn't terminated yet
	if (b.White.Queens|b.White.Rooks|b.White.Pawns) != 0 ||
		(b.Black.Queens|b.Black.Rooks|b.Black.Pawns) != 0 {
		return false
	}

	// If there is only 1 or less knight and 1 or less bishop, game is a draw
	if oneOrLess(b.White.Bishops, b.White.Knights) || oneOrLess(b.Black.Bishops, b.Black.Knights) {
		b.termination = TerminationInsufficientMaterial
		return true
	}

	return false
}

func (b Board) Clone() *Board {
	history := make([]History, len(b.history))
	copy(history, b.history)
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
		history:         history,
		termination:     b.termination,
		irreversibleIdx: b.irreversibleIdx,
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
func (b *Board) whiteCanCastleQueenside() bool {
	return b.castlerights&1 == 1
}
func (b *Board) whiteCanCastleKingside() bool {
	return (b.castlerights&0x2)>>1 == 1
}
func (b *Board) blackCanCastleQueenside() bool {
	return (b.castlerights&0x4)>>2 == 1
}
func (b *Board) blackCanCastleKingside() bool {
	return (b.castlerights&0x8)>>3 == 1
}
func (b *Board) canCastleQueenside() bool {
	if b.Wtomove {
		return b.whiteCanCastleQueenside()
	} else {
		return b.blackCanCastleQueenside()
	}
}
func (b *Board) canCastleKingside() bool {
	if b.Wtomove {
		return b.whiteCanCastleKingside()
	} else {
		return b.blackCanCastleKingside()
	}
}
func (b *Board) oppCanCastleQueenside() bool {
	if b.Wtomove {
		return b.blackCanCastleQueenside()
	} else {
		return b.whiteCanCastleQueenside()
	}
}
func (b *Board) oppCanCastleKingside() bool {
	if b.Wtomove {
		return b.blackCanCastleKingside()
	} else {
		return b.whiteCanCastleKingside()
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
