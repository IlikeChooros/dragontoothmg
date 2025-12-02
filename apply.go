package dragontoothmg

// Applies a move to the board, and returns a function that can be used to unapply it.
// This function assumes that the given move is valid (i.e., is in the set of moves found by GenerateLegalMoves()).
// If the move is not valid, this function has undefined behavior.
func (b *Board) Apply(m Move) func() {
	b.Make(m)

	// For backwards compatibility
	// Return the unapply function (closure)
	unapply := func() {
		b.Undo()
	}
	return unapply
}

// Makes a move on the board. This function assumes that the given move is valid (i.e., is in the set of moves found by GenerateLegalMoves()).
// If the move is not valid, this function has undefined behavior.
func (b *Board) Make(m Move) {

	// Configure data about which pieces move
	hashBefore := b.hash
	var ourBitboardPtr, oppBitboardPtr *Bitboards
	var epDelta int8                                // add this to the e.p. square to find the captured pawn
	var oppStartingRankBb, ourStartingRankBb uint64 // the starting rank of out opponent's major pieces
	// the constant that represents the index into pieceSquareZobristC for the pawn of our color
	var ourPiecesPawnZobristIndex int
	var oppPiecesPawnZobristIndex int
	if b.Wtomove {
		ourBitboardPtr = &(b.White)
		oppBitboardPtr = &(b.Black)
		epDelta = -8
		oppStartingRankBb = onlyRank[7]
		ourStartingRankBb = onlyRank[0]
		ourPiecesPawnZobristIndex = 0
		oppPiecesPawnZobristIndex = 6
	} else {
		ourBitboardPtr = &(b.Black)
		oppBitboardPtr = &(b.White)
		epDelta = 8
		oppStartingRankBb = onlyRank[0]
		ourStartingRankBb = onlyRank[7]
		b.Fullmoveno++ // increment after black's move
		ourPiecesPawnZobristIndex = 6
		oppPiecesPawnZobristIndex = 0
	}
	fromBitboard := (uint64(1) << m.From())
	toBitboard := (uint64(1) << m.To())
	pieceType, pieceTypeBitboard := determinePieceType(ourBitboardPtr, fromBitboard)
	castleStatus := 0

	var oldRookLoc, newRookLoc uint8
	var flippedKsCastle, flippedQsCastle, flippedOppKsCastle, flippedOppQsCastle bool

	// If it is any kind of capture or pawn move, reset halfmove clock.
	resetHalfmoveClockFrom := -1
	if IsCapture(m, b) || pieceType == Pawn {
		resetHalfmoveClockFrom = int(b.Halfmoveclock)
		b.Halfmoveclock = 0 // reset halfmove clock
		b.irreversibleIdx = len(b.history) - 1
	} else {
		b.Halfmoveclock++
	}

	// King moves strip castling rights
	if pieceType == King {
		// TODO(dylhunn): do this without a branch
		if m.To()-m.From() == 2 { // castle short
			castleStatus = 1
			oldRookLoc = m.To() + 1
			newRookLoc = m.To() - 1
		} else if int(m.To())-int(m.From()) == -2 { // castle long
			castleStatus = -1
			oldRookLoc = m.To() - 2
			newRookLoc = m.To() + 1
		}
		// King moves always strip castling rights
		if b.canCastleKingside() {
			b.flipKingsideCastle()
			flippedKsCastle = true
		}
		if b.canCastleQueenside() {
			b.flipQueensideCastle()
			flippedQsCastle = true
		}
	}

	// Rook moves strip castling rights
	if pieceType == Rook {
		if b.canCastleKingside() && (fromBitboard&onlyFile[7] != 0) &&
			fromBitboard&ourStartingRankBb != 0 { // king's rook
			flippedKsCastle = true
			b.flipKingsideCastle()
		} else if b.canCastleQueenside() && (fromBitboard&onlyFile[0] != 0) &&
			fromBitboard&ourStartingRankBb != 0 { // queen's rook
			flippedQsCastle = true
			b.flipQueensideCastle()
		}
	}

	// Apply the castling rook movement
	if castleStatus != 0 {
		ourBitboardPtr.Rooks |= (uint64(1) << newRookLoc)
		ourBitboardPtr.All |= (uint64(1) << newRookLoc)
		ourBitboardPtr.Rooks &= ^(uint64(1) << oldRookLoc)
		ourBitboardPtr.All &= ^(uint64(1) << oldRookLoc)
		// Update rook location in hash
		// (Rook - 1) assumes that "Nothing" precedes "Rook" in the Piece constants list
		b.hash ^= pieceSquareZobristC[ourPiecesPawnZobristIndex+(Rook-1)][oldRookLoc]
		b.hash ^= pieceSquareZobristC[ourPiecesPawnZobristIndex+(Rook-1)][newRookLoc]
	}

	// Is this an e.p. capture? Strip the opponent pawn and reset the e.p. square
	oldEpCaptureSquare := b.enpassant
	var actuallyPerformedEpCapture bool = false
	if pieceType == Pawn && m.To() == oldEpCaptureSquare && oldEpCaptureSquare != 0 {
		actuallyPerformedEpCapture = true
		epOpponentPawnLocation := uint8(int8(oldEpCaptureSquare) + epDelta)
		oppBitboardPtr.Pawns &= ^(uint64(1) << epOpponentPawnLocation)
		oppBitboardPtr.All &= ^(uint64(1) << epOpponentPawnLocation)
		// Remove the opponent pawn from the board hash.
		b.hash ^= pieceSquareZobristC[oppPiecesPawnZobristIndex][epOpponentPawnLocation]
	}
	// Update the en passant square
	if pieceType == Pawn && (int8(m.To())+2*epDelta == int8(m.From())) { // pawn double push
		b.enpassant = uint8(int8(m.To()) + epDelta)
	} else {
		b.enpassant = 0
	}

	// Is this a promotion?
	var destTypeBitboard *uint64
	var promotedToPieceType Piece // if not promoted, same as pieceType
	switch m.Promote() {
	case Queen:
		destTypeBitboard = &(ourBitboardPtr.Queens)
		promotedToPieceType = Queen
	case Knight:
		destTypeBitboard = &(ourBitboardPtr.Knights)
		promotedToPieceType = Knight
	case Rook:
		destTypeBitboard = &(ourBitboardPtr.Rooks)
		promotedToPieceType = Rook
	case Bishop:
		destTypeBitboard = &(ourBitboardPtr.Bishops)
		promotedToPieceType = Bishop
	default:
		destTypeBitboard = pieceTypeBitboard
		promotedToPieceType = pieceType
	}

	// Apply the move
	capturedPieceType, capturedBitboard := determinePieceType(oppBitboardPtr, toBitboard)
	ourBitboardPtr.All &= ^fromBitboard // remove at "from"
	ourBitboardPtr.All |= toBitboard    // add at "to"
	*pieceTypeBitboard &= ^fromBitboard // remove at "from"
	*destTypeBitboard |= toBitboard     // add at "to"
	if capturedPieceType != Nothing {   // This does not account for e.p. captures
		*capturedBitboard &= ^toBitboard
		oppBitboardPtr.All &= ^toBitboard
		b.hash ^= pieceSquareZobristC[oppPiecesPawnZobristIndex+(int(capturedPieceType)-1)][m.To()] // remove the captured piece from the hash
	}
	b.hash ^= pieceSquareZobristC[(int(pieceType)-1)+ourPiecesPawnZobristIndex][m.From()]         // remove piece at "from"
	b.hash ^= pieceSquareZobristC[(int(promotedToPieceType)-1)+ourPiecesPawnZobristIndex][m.To()] // add piece at "to"

	// If a rook was captured, it strips castling rights
	if capturedPieceType == Rook {
		if m.To()%8 == 7 && toBitboard&oppStartingRankBb != 0 && b.oppCanCastleKingside() { // captured king rook
			b.flipOppKingsideCastle()
			flippedOppKsCastle = true
		} else if m.To()%8 == 0 && toBitboard&oppStartingRankBb != 0 && b.oppCanCastleQueenside() { // queen rooks
			b.flipOppQueensideCastle()
			flippedOppQsCastle = true
		}
	}
	// flip the side to move in the hash
	b.hash ^= whiteToMoveZobristC
	b.Wtomove = !b.Wtomove

	// remove the old en passant square from the hash, and add the new one
	b.hash ^= uint64(oldEpCaptureSquare)
	b.hash ^= uint64(b.enpassant)

	// Set all undo fields, there are probably some redundant ones here
	// but it makes the code simpler to read and write
	// (and uses negligible memory)
	h := History{}
	h.resetHalfmoveClockFrom = resetHalfmoveClockFrom
	h.actuallyPerformedEpCapture = actuallyPerformedEpCapture
	h.capturedBitboard = capturedBitboard
	h.capturedPieceType = capturedPieceType
	h.castleStatus = castleStatus
	h.destTypeBitboard = destTypeBitboard
	h.flippedKsCastle = flippedKsCastle
	h.flippedQsCastle = flippedQsCastle
	h.flippedOppKsCastle = flippedOppKsCastle
	h.flippedOppQsCastle = flippedOppQsCastle
	h.m = m
	h.newRookLoc = newRookLoc
	h.oldEpCaptureSquare = oldEpCaptureSquare
	h.oldRookLoc = oldRookLoc
	h.pieceType = pieceType
	h.pieceTypeBitboard = pieceTypeBitboard
	h.promotedToPieceType = promotedToPieceType
	h.resetHalfmoveClockFrom = resetHalfmoveClockFrom
	h.hashBefore = hashBefore
	h.hashCurrent = b.hash

	b.history = append(b.history, h)
}

// Undoes the last move. If there is no move to undo, this function does nothing.
// It may be called multiple times in succession to undo multiple moves.
func (b *Board) Undo() {
	if len(b.history) <= 1 {
		return
	}
	b.termination = TerminationNone
	u := &b.history[len(b.history)-1]

	// Configure data about which pieces move
	var ourBitboardPtr, oppBitboardPtr *Bitboards
	var epDelta int8 // add this to the e.p. square to find the captured pawn
	// the constant that represents the index into pieceSquareZobristC for the pawn of our color
	if !b.Wtomove {
		ourBitboardPtr = &(b.White)
		oppBitboardPtr = &(b.Black)
		epDelta = -8
	} else {
		ourBitboardPtr = &(b.Black)
		oppBitboardPtr = &(b.White)
		epDelta = 8
	}

	// Flip the player to move
	b.Wtomove = !b.Wtomove

	// Restore the halfmove clock
	if u.resetHalfmoveClockFrom == -1 {
		b.Halfmoveclock--
	} else {
		b.Halfmoveclock = uint8(u.resetHalfmoveClockFrom)
	}

	fromBitboard := uint64(1) << u.m.From()
	toBitboard := uint64(1) << u.m.To()

	// Unapply move
	ourBitboardPtr.All &= ^toBitboard    // remove at "to"
	ourBitboardPtr.All |= fromBitboard   // add at "from"
	*u.destTypeBitboard &= ^toBitboard   // remove at "to"
	*u.pieceTypeBitboard |= fromBitboard // add at "from"
	// Restore captured piece (excluding e.p.)
	if u.capturedPieceType != Nothing { // doesn't consider e.p. captures
		*u.capturedBitboard |= toBitboard
		oppBitboardPtr.All |= toBitboard
	}

	// Restore rooks from castling move
	if u.castleStatus != 0 {
		ourBitboardPtr.Rooks &= ^(uint64(1) << u.newRookLoc)
		ourBitboardPtr.All &= ^(uint64(1) << u.newRookLoc)
		ourBitboardPtr.Rooks |= (uint64(1) << u.oldRookLoc)
		ourBitboardPtr.All |= (uint64(1) << u.oldRookLoc)
	}

	// Unapply en-passant square change, and capture if necessary
	b.enpassant = u.oldEpCaptureSquare
	if u.actuallyPerformedEpCapture {
		epOpponentPawnLocation := uint8(int8(u.oldEpCaptureSquare) + epDelta)
		oppBitboardPtr.Pawns |= (uint64(1) << epOpponentPawnLocation)
		oppBitboardPtr.All |= (uint64(1) << epOpponentPawnLocation)
	}

	// Decrement move clock
	if !b.Wtomove {
		b.Fullmoveno-- // decrement after undoing black's move
	}

	// Restore castling flags
	// Must update castling flags AFTER turn swap
	if u.flippedKsCastle {
		b.flipKingsideCastle()
	}
	if u.flippedQsCastle {
		b.flipQueensideCastle()
	}
	if u.flippedOppKsCastle {
		b.flipOppKingsideCastle()
	}
	if u.flippedOppQsCastle {
		b.flipOppQueensideCastle()
	}

	// Reset the hash and reslice the history
	b.hash = u.hashBefore
	b.history = b.history[:len(b.history)-1]
}

// Make null move - pass the turn to the opponent side, must be undone with UndoNullMove(),
// it is allowed to make consecutive null moves
func (b *Board) MakeNullMove() {
	hashBefore := b.hash

	// If this position has an enpassant square, remove it
	b.hash ^= uint64(b.enpassant)
	oldEpCaptureSquare := b.enpassant
	b.enpassant = 0

	// Flip the sides
	b.Wtomove = !b.Wtomove
	b.hash ^= whiteToMoveZobristC

	b.history = append(b.history,
		History{hashBefore: hashBefore, oldEpCaptureSquare: oldEpCaptureSquare})
}

func (b *Board) UndoNullMove() {
	if len(b.history) == 0 {
		return
	}

	u := &b.history[len(b.history)-1]

	// Restore previous state
	b.Wtomove = !b.Wtomove
	b.enpassant = u.oldEpCaptureSquare
	b.hash = u.hashBefore

	// Slice the history
	b.history = b.history[:len(b.history)-1]
}

func determinePieceType(ourBitboardPtr *Bitboards, squareMask uint64) (Piece, *uint64) {
	var pieceType Piece = Nothing
	pieceTypeBitboard := &(ourBitboardPtr.All)
	if squareMask&ourBitboardPtr.Pawns != 0 {
		pieceType = Pawn
		pieceTypeBitboard = &(ourBitboardPtr.Pawns)
	} else if squareMask&ourBitboardPtr.Knights != 0 {
		pieceType = Knight
		pieceTypeBitboard = &(ourBitboardPtr.Knights)
	} else if squareMask&ourBitboardPtr.Bishops != 0 {
		pieceType = Bishop
		pieceTypeBitboard = &(ourBitboardPtr.Bishops)
	} else if squareMask&ourBitboardPtr.Rooks != 0 {
		pieceType = Rook
		pieceTypeBitboard = &(ourBitboardPtr.Rooks)
	} else if squareMask&ourBitboardPtr.Queens != 0 {
		pieceType = Queen
		pieceTypeBitboard = &(ourBitboardPtr.Queens)
	} else if squareMask&ourBitboardPtr.Kings != 0 {
		pieceType = King
		pieceTypeBitboard = &(ourBitboardPtr.Kings)
	}
	return pieceType, pieceTypeBitboard
}
