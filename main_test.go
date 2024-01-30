package extension_patcher

import (
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

func Test_mustReplaceExhaustiveStr(t *testing.T) {
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mustReplaceNStr(tt.args.in, tt.args.old, tt.args.new, tt.args.n); got != tt.want {
				t.Errorf("mustReplaceExhaustiveStr() = %v, want %v", got, tt.want)
			}
		})
	}
}
