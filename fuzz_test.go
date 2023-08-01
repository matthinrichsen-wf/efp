//go:build go1.18
// +build go1.18

package efp_test

import (
	"testing"

	"github.com/xuri/efp"
)

func FuzzParse(f *testing.F) {
	f.Add("=0")
	f.Add("=SUM(A3+B9*2)/2")
	f.Fuzz(func(t *testing.T, formula string) {
		p := efp.ExcelParser()
		tokens := p.Parse(formula)
		_ = tokens
		if p.InError {
			t.Skip()
		}
		t.Log(p.Render())
	})
}

var benchFormulas = []string{"=0", "=SUM(A3+B9*2)/2"}

func BenchmarkParse(b *testing.B) {
	for _, formula := range benchFormulas {
		ps := efp.ExcelParser()
		b.Run(formula, func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				ps.Parse(formula)
			}
		})
	}
}
