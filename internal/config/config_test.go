// SPDX-License-Identifier: MIT

package config

import (
	"testing"
	"time"

	"github.com/spf13/cobra"
)

type Inner struct {
	Name string `flag:"name" env:"T_NAME" desc:"a name"`
}

type cfg struct {
	Inner
	B   bool          `flag:"b" env:"T_B" desc:"b"`
	U8  uint8         `flag:"u8" env:"T_U8" desc:"u8"`
	U16 uint16        `flag:"u16" env:"T_U16" desc:"u16"`
	U32 uint32        `flag:"u32" env:"T_U32" desc:"u32"`
	U64 uint64        `flag:"u64" env:"T_U64" desc:"u64"`
	U   uint          `flag:"u" env:"T_U" desc:"u"`
	I   int           `flag:"i" env:"T_I" desc:"i"`
	I8  int8          `flag:"i8" env:"T_I8" desc:"i8"`
	I16 int16         `flag:"i16" env:"T_I16" desc:"i16"`
	I32 int32         `flag:"i32" env:"T_I32" desc:"i32"`
	I64 int64         `flag:"i64" env:"T_I64" desc:"i64"`
	F   float64       `flag:"f" env:"T_F" desc:"f"`
	D   time.Duration `flag:"d" env:"T_D" desc:"d"`
	S   []string      `flag:"s" env:"T_S" desc:"s"`
}

func TestEnvAndFlags(t *testing.T) {
	for k, v := range map[string]string{"T_NAME": "n", "T_B": "true", "T_U8": "8", "T_U16": "16", "T_U32": "32", "T_U64": "64",
		"T_U": "1", "T_I": "-1", "T_I8": "-8", "T_I16": "-16", "T_I32": "-32", "T_I64": "-64", "T_F": "1.5", "T_D": "90s", "T_S": "a,b"} {
		t.Setenv(k, v)
	}
	var c cfg
	if err := LoadFromEnv(&c); err != nil {
		t.Fatal(err)
	}
	if c.Name != "n" || !c.B || c.U8 != 8 || c.U64 != 64 || c.I64 != -64 || c.F != 1.5 || c.D != 90*time.Second || len(c.S) != 2 {
		t.Errorf("LoadFromEnv: %+v", c)
	}
	cmd := &cobra.Command{Use: "x"}
	RegisterFlags(cmd, &c)
	if err := cmd.ParseFlags([]string{"--d", "1m", "--name", "m", "--i8", "3"}); err != nil {
		t.Fatal(err)
	}
	if c.D != time.Minute || c.Name != "m" || c.I8 != 3 {
		t.Errorf("flags: %+v", c)
	}
	for _, k := range []string{"T_B", "T_U8", "T_I", "T_F", "T_D", "T_U16"} {
		t.Setenv(k, "junk")
	}
	if err := LoadFromEnv(&cfg{}); err == nil {
		t.Error("LoadFromEnv of junk")
	}
	defer func() {
		if recover() == nil {
			t.Error("MustLoadFromEnv of junk did not panic")
		}
	}()
	MustLoadFromEnv(&cfg{})
}
