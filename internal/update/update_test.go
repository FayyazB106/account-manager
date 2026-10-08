package update

import "testing"

func TestIsNewer(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"1.2.4", "1.2.3", true},
		{"1.3.0", "1.2.9", true},
		{"2.0.0", "1.9.9", true},
		{"1.2.3", "1.2.3", false},
		{"1.2.3", "1.2.4", false},
		{"v1.2.4", "v1.2.3", true}, // leading v tolerated
		{"1.2", "1.1", true},
		{"1.2.3", "dev", false},    // unstamped build never nags
		{"", "1.2.3", false},       // no release
		{"1.2.3-rc1", "1.2.3", false},
		{"1.2.4-rc1", "1.2.3", true}, // pre-release metadata ignored for compare
		{"garbage", "1.2.3", false},
	}
	for _, c := range cases {
		if got := isNewer(c.latest, c.current); got != c.want {
			t.Errorf("isNewer(%q,%q)=%v want %v", c.latest, c.current, got, c.want)
		}
	}
}

func TestSumFor(t *testing.T) {
	sums := "" +
		"aaaa1111  account-manager-linux-amd64.tar.gz\n" +
		"bbbb2222 *account-manager-setup.exe\n" + // '*' binary marker tolerated
		"cccc3333  account-manager-macos.dmg\n"

	if got := sumFor(sums, "account-manager-setup.exe"); got != "bbbb2222" {
		t.Errorf("setup.exe = %q want bbbb2222", got)
	}
	if got := sumFor(sums, "account-manager-macos.dmg"); got != "cccc3333" {
		t.Errorf("dmg = %q want cccc3333", got)
	}
	if got := sumFor(sums, "does-not-exist"); got != "" {
		t.Errorf("missing = %q want empty", got)
	}
}
