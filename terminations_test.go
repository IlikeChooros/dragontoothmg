package dragontoothmg

import "testing"

func TestTerminations(t *testing.T) {
	fens := []string{
		"8/8/8/8/8/4k3/8/r3K3 w - - 6 4",
		"4k3/4P3/4K3/8/8/8/8/8 b - - 0 1",
		"7k/ppp5/8/8/8/8/7K/8 w - - 100 1",

		"4k3/8/8/5KB1/8/8/8/8 w - - 0 1",
		"4k3/8/8/5K2/8/8/8/8 w - - 0 1",
		"4k3/8/8/5KN1/8/8/8/8 w - - 0 1",
		"4kn2/8/8/5KB1/8/8/8/8 w - - 0 1",
		"4kb2/8/8/5KB1/8/8/8/8 w - - 0 1",
	}

	terminations := []Termination{
		TerminationCheckmate,
		TerminationStalemate,
		TerminationFiftyMovesRule,

		TerminationInsufficientMaterial,
		TerminationInsufficientMaterial,
		TerminationInsufficientMaterial,
		TerminationInsufficientMaterial,
		TerminationInsufficientMaterial,
	}

	for i := range fens {
		b, ok := FromFen(fens[i])

		if !ok {
			t.Errorf("Invalid fen string %s", fens[i])
			continue
		}

		moves := b.GenerateLegalMoves()
		if !b.IsTerminated(len(moves)) || b.Termination() != terminations[i] {
			t.Errorf("Invalid termination reason %v, want %v", b.Termination(), terminations[i])
		}
	}
}

func TestRepetitions(t *testing.T) {
	fens := []string{
		"8/8/8/r7/8/7K/2k5/8 w - - 0 1",
		"8/8/8/r7/8/7K/2k5/8 b - - 0 1",
		"8/5pk1/6p1/8/4Q3/8/5K2/8 w - - 0 1",
		"8/6k1/8/8/4QR2/8/5K2/8 w - - 0 1",
		"8/6k1/5qr1/8/8/8/6K1/8 w - - 0 1",
	}

	moves := []string{
		"h3g3 a5a4 g3h3 a4a5 h3g3 a5a4 g3h3 a4a5",
		"a5a4 h3g3 a4a5 g3h3 a5a4 h3g3 a4a5 g3h3",
		"e4e5 g7g8 e5e8 g8g7 e8e5 g7g8 e5e8 g8g7 e8e5",
		"f4h4 g7g8 h4f4 g8g7 f4h4 g7g8 h4f4 g8g7",
		"g2h3 g6h6 h3g2 h6g6 g2h3 g6h6 h3g2 h6g6",
	}

	for i := range fens {
		b, ok := FromFen(fens[i])

		if !ok {
			t.Errorf("Invalid fen string %s", fens[i])
			continue
		}

		mvs, err := ParseMoves(moves[i])
		if err != nil {
			t.Error(err)
			continue
		}

		for j := range mvs {
			b.Make(mvs[j])
		}

		if !b.IsRepetition(3) {
			t.Errorf("%s is not a repetition", fens[i])
		}
	}
}
