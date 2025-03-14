package extension_patcher

import (
	"bytes"
	"os"
	"reflect"
	"testing"
)

func Test_mustReplaceStr(t *testing.T) {
	type args struct {
		in  string
		old string
		new string
		n   int
	}
	tests := []struct {
		name    string
		args    args
		wantOut string
	}{
		{name: "1", args: args{in: "ToReplace ToReplace ToReplace", old: "ToReplace", new: "ToReplaceNew", n: 3}, wantOut: "ToReplaceNew ToReplaceNew ToReplaceNew"},
		{name: "1", args: args{in: "ToReplace ToReplace Some Other Text", old: "ToReplace", new: "ToReplaceNew", n: 2}, wantOut: "ToReplaceNew ToReplaceNew Some Other Text"},
		{name: "1", args: args{in: "Some Other Text ToReplace ToReplace Some Other Text", old: "ToReplace", new: "ToReplaceNew", n: 2}, wantOut: "Some Other Text ToReplaceNew ToReplaceNew Some Other Text"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotOut := mustReplaceStr(tt.args.in, tt.args.old, tt.args.new, tt.args.n); !reflect.DeepEqual(gotOut, tt.wantOut) {
				t.Errorf("mustReplace() = %v, want %v", gotOut, tt.wantOut)
			}
		})
	}
}

func Test_mustReplaceNStr(t *testing.T) {
	type args struct {
		in  string
		old string
		new string
		n   int
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{name: "1", args: args{in: "ToReplace ToReplace ToReplace", old: "ToReplace", new: "ToReplaceNew", n: 3}, want: "ToReplaceNew ToReplaceNew ToReplaceNew"},
		{name: "1", args: args{in: "ToReplace Or ToReplace Some Other Text", old: "ToReplace", new: "ToReplaceNew", n: 2}, want: "ToReplaceNew Or ToReplaceNew Some Other Text"},
		{name: "1", args: args{in: "ToReplace", old: "ToReplace", new: "New {old} New", n: 1}, want: "New ToReplace New"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mustReplaceNStr(tt.args.in, tt.args.old, tt.args.new, tt.args.n); got != tt.want {
				t.Errorf("mustReplaceNStr() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProviders(t *testing.T) {
	t.Skip("Skip provider tests")

	t.Run("Test File provider", func(t *testing.T) {
		defer os.RemoveAll("./test")
		MustNew(Params{
			DelayBeforeClose: Int(0),
			ExtensionName:    "test",
			ExpectedSha256:   "4c2e9e6da31a64c70623619c449a040968cdbea85945bf384fa30ed2d5d24fa3",
			Uri:              "./testdata/test1.js",
			KeepOriginal:     true,
			Files: []FileAndProcessors{
				NewFile("/test1.js", func(by []byte) []byte { return by }),
			},
		}).Start()
		by, _ := os.ReadFile("./test/test1.js")
		if string(by) != "Some text" {
			t.Fail()
		}
	})

	t.Run("Test ChromeWebstore provider", func(t *testing.T) {
		defer os.RemoveAll("./universeview-extension")
		MustNew(Params{
			DelayBeforeClose: Int(0),
			ExpectedSha256:   "84d99167220fc7563b4ebb925f80b1e65d9420ea4a2d16ccccebafbe7d2da259",
			Uri:              "https://chromewebstore.google.com/detail/universeview-extension/ipmfkhoihjbbohnfecpmhekhippaplnh",
			Files: []FileAndProcessors{
				NewFile("manifest.json", func(by []byte) []byte {
					return MustReplaceN(by, "UniverseView Extension", "UniverseViewNinja Extension", 1)
				}),
			},
		}).Start()
		by, _ := os.ReadFile("./universeview-extension/manifest.json")
		if !bytes.Contains(by, []byte("UniverseViewNinja Extension")) {
			t.Fail()
		}
	})

	t.Run("Test OpenUserJS provider", func(t *testing.T) {
		defer os.RemoveAll("./OGLight")
		MustNew(Params{
			DelayBeforeClose: Int(0),
			ExpectedSha256:   "978c7932426a23b2c69d7f4be32ea4e8e3abbb6a3ea84d7278381be6336f55c3",
			Uri:              "https://openuserjs.org/install/nullNaN/OGLight.user.js",
			KeepOriginal:     true,
			Files: []FileAndProcessors{
				NewFile("OGLight.user.js", func(by []byte) []byte {
					return MustReplaceN(by, "// @name         OGLight", "// @name         OGLightNinja", 1)
				}),
			},
		}).Start()
		by, _ := os.ReadFile("./OGLight/OGLight.user.js")
		if !bytes.Contains(by, []byte("OGLightNinja")) {
			t.Fail()
		}
	})

	t.Run("Test MozillaWebstore provider", func(t *testing.T) {
		defer os.RemoveAll("./ogame-infinity")
		MustNew(Params{
			DelayBeforeClose: Int(0),
			ExpectedSha256:   "b62c9abd2b60c6447f4bbbcc00066bae82b76308ca9ac4990c3f448b5b6dc83c",
			Uri:              "https://addons.mozilla.org/en-US/firefox/addon/ogame-infinity",
			KeepOriginal:     false,
			Files: []FileAndProcessors{
				NewFile("/manifest.json", func(by []byte) []byte {
					return MustReplaceN(by, `"name": "Ogame Infinity"`, `"name": "Ogame Infinity Ninja"`, 1)
				}),
			},
		}).Start()
		by, _ := os.ReadFile("./ogame-infinity/manifest.json")
		if !bytes.Contains(by, []byte("Ogame Infinity Ninja")) {
			t.Fail()
		}
	})
}
