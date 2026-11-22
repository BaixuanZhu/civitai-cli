package app

import "testing"

func TestBootstrapEnabled_Behavior(t *testing.T) {
	cases := []struct {
		name      string
		bootstrap string
		exe       string
		noBootEnv string
		tempDir   string
		want      bool
	}{
		{"release build normal path", "on", `C:\Tools\civ.exe`, "", `C:\Temp`, true},
		{"env switch disables", "on", `C:\Tools\civ.exe`, "1", `C:\Temp`, false},
		{"env whitespace-only treated as unset", "on", `C:\Tools\civ.exe`, "  ", `C:\Temp`, true},
		{"ldflags off disables", "off", `C:\Tools\civ.exe`, "", `C:\Temp`, false},
		{"exe under temp disables", "on", `C:\Temp\go-build123\civ.exe`, "", `C:\Temp`, false},
		{"exe is temp dir itself disables", "on", `C:\Temp`, "", `C:\Temp`, false},
		{"exe path empty treated as non-temp", "on", "", "", `C:\Temp`, true},
		{"case insensitive temp match", "on", `c:\temp\civ.exe`, "", `C:\Temp`, false},
		{"sibling prefix not under temp", "on", `C:\TempOther\civ.exe`, "", `C:\Temp`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			old := Bootstrap
			Bootstrap = tc.bootstrap
			defer func() { Bootstrap = old }()
			if got := bootstrapEnabled(tc.exe, tc.noBootEnv, tc.tempDir); got != tc.want {
				t.Errorf("bootstrapEnabled(%q,%q,%q) Bootstrap=%q = %v, want %v",
					tc.exe, tc.noBootEnv, tc.tempDir, tc.bootstrap, got, tc.want)
			}
		})
	}
}

func TestIsUnderTemp_Behavior(t *testing.T) {
	cases := []struct {
		name    string
		exe     string
		tempDir string
		want    bool
	}{
		{"direct child", `C:\Temp\civ.exe`, `C:\Temp`, true},
		{"nested child", `C:\Temp\a\b\civ.exe`, `C:\Temp`, true},
		{"temp dir itself", `C:\Temp`, `C:\Temp`, true},
		{"unrelated", `C:\Tools\civ.exe`, `C:\Temp`, false},
		{"prefix sibling", `C:\TempX\civ.exe`, `C:\Temp`, false},
		{"trailing separator on temp", `C:\Temp\civ.exe`, `C:\Temp\`, true},
		{"case insensitive", `C:\TEMP\CIV.EXE`, `c:\temp`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isUnderTemp(tc.exe, tc.tempDir); got != tc.want {
				t.Errorf("isUnderTemp(%q,%q) = %v, want %v", tc.exe, tc.tempDir, got, tc.want)
			}
		})
	}
}
