package run

import "testing"

func TestFiguresAreWrittenInTheirFormat(t *testing.T) {
	one := 1
	eur := numberFormat{kind: "currency", symbol: "€", decimals: -1}
	pct := numberFormat{kind: "percent", decimals: -1}
	rating := numberFormat{kind: "number", decimals: one}
	cases := []struct {
		got, want string
	}{
		{eur.short(6150028.97), "€6.2M"},
		{eur.short(982158.38), "€982,158"},
		{eur.short(32.5), "€32.50"},
		{eur.short(0), "€0"},
		{eur.short(-176696.2), "-€176,696"},
		{eur.exact(982158.38), "€982,158.38"},
		{eur.tick(200000), "€200,000"},
		{pct.short(0.873), "87.3%"},
		{pct.short(0.85), "85%"},
		{pct.tick(0.5), "50%"},
		{rating.short(4.06), "4.1"},
		{rating.tick(50), "50"},
		{rating.tick(2.5), "2.5"},
		{plainFormat.short(982158.38), "982,158.38"},
		{numberFormat{kind: "currency", symbol: "CHF ", decimals: -1}.short(1200), "CHF 1,200"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("got %q, want %q", c.got, c.want)
		}
	}
	if s, _ := pct.change(0.85, 0.82); s != "+3.0 pts" {
		t.Errorf("a share moved by %q, want +3.0 pts", s)
	}
}
