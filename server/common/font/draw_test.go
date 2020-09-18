package font

import (
	"os"
	"testing"
)

func TestDraw(t *testing.T) {
	tb := &Textbox{Width: 100, LineSpace: 4, CharSpace: 0, Margin: 8}
	tb.Begin()
	tb.Underline = true
	tb.Write("asdfsadfsadfsdf  asdf asd fasd fasd fsa\n第二哈asdf ")
	tb.Gray = true
	tb.Underline = false
	tb.Write("\ngray\ttext")
	of, _ := os.Create("1.png")
	tb.End(of)
}
