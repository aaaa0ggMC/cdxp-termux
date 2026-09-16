package cdxp

import "testing"

// The expected strings are the literal output of bash's `printf '%q'`.
func TestShellQuoteMatchesBashQ(t *testing.T) {
	cases := map[string]string{
		"":                          "''",
		"plain-word":                "plain-word",
		"sk-8c2…fa52":               "sk-8c2…fa52",
		"中文":                        "中文",
		`model_provider="deepseek"`: `model_provider=\"deepseek\"`,
		"a b":                       `a\ b`,
		"it's":                      `it\'s`,
		"x;rm -rf /":                `x\;rm\ -rf\ /`,
		`back\slash`:                `back\\slash`,
		"star*":                     `star\*`,
		"quest?":                    `quest\?`,
		"dollar$HOME":               `dollar\$HOME`,
		"tilde~":                    "tilde~",
		"hash#":                     "hash#",
		"bang!":                     `bang\!`,
		"paren()":                   `paren\(\)`,
		"brace{}":                   `brace\{\}`,
		"brack[]":                   `brack\[\]`,
		"amp&":                      `amp\&`,
		"pipe|":                     `pipe\|`,
		"less<":                     `less\<`,
		"caret^":                    `caret\^`,
		"eq=a":                      "eq=a",
		"at@b":                      "at@b",
		"pct%":                      "pct%",
		"colon:x":                   "colon:x",
		"plus+":                     "plus+",
		"a\tb":                      `$'a\tb'`,
		"a\nb":                      `$'a\nb'`,
		"a\rb":                      `$'a\rb'`,
		"a\x01b":                    `$'a\001b'`,
	}
	for in, want := range cases {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMaskKey(t *testing.T) {
	cases := map[string]string{
		"":                              "****",
		"short":                         "****",
		"exactly12chr":                  "****",
		"thirteen_chars":                "thirte…hars",
		"sk-8c2a13d4e5f6a7b8c9d0e1fa52": "sk-8c2…fa52",
	}
	for in, want := range cases {
		if got := maskKey(in); got != want {
			t.Errorf("maskKey(%q) = %q, want %q", in, got, want)
		}
	}
}

// padRight counts bytes, because bash's printf %-*s does.
func TestPadRightCountsBytes(t *testing.T) {
	if got, want := padRight("auth.hint", 12), "auth.hint   "; got != want {
		t.Errorf("padRight(ascii) = %q, want %q", got, want)
	}
	if got, want := padRight("已解析", 12), "已解析   "; got != want {
		t.Errorf("padRight(cjk) = %q, want %q", got, want)
	}
	if got, want := padRight("toolongvalue", 4), "toolongvalue"; got != want {
		t.Errorf("padRight(overflow) = %q, want %q", got, want)
	}
}
