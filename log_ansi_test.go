package main

import "testing"

func TestPlainLogText(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"\x1b[32m\x1b[1mVITE\x1b[22m v6.4.3\x1b[39m", "VITE v6.4.3"},
		{"\x1b]0;title\x07\x1b[2K中文\x1b]8;;https://example.invalid\x1b\\地址\x1b]8;;\x1b\\", "中文地址"},
		{"\u009b31m错误\u009b0m\x00\x1b[31;", "错误"},
		{"正常\t日志 <img onerror=alert(1)>", "正常\t日志 <img onerror=alert(1)>"},
	} {
		if got := plainLogText(tc.input); got != tc.want {
			t.Errorf("plainLogText(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
