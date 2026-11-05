// SPDX-License-Identifier: MIT

package config

import (
	"fmt"
	"reflect"
	"time"

	"github.com/spf13/cobra"
)

// RegisterFlags binds cobra flags to tagged fields on cfg (pointer to struct).
// An embedded struct of flags must be of an exported type: reflect cannot
// take the address of the fields of an unexported one.
// Fields are bound through a *T of their kind with the matching pflag *Var function,
// converted with reflect (no unsafe.Pointer casts), so that a field of a named type
// (type Port uint16) binds as its kind does instead of panicking on a type assertion.
// Supported kinds: string, bool, []string, uint8–uint64, int, int8–int64, uint, float64
// and time.Duration.
func RegisterFlags(cmd *cobra.Command, cfg any) {
	v := reflect.ValueOf(cfg).Elem()
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		flagName := field.Tag.Get("flag")
		desc := field.Tag.Get("desc")
		envVar := field.Tag.Get("env")

		if field.Anonymous {
			RegisterFlags(cmd, v.Field(i).Addr().Interface())
			continue
		}

		if flagName == "" || flagName == "-" {
			continue
		}

		if envVar != "" {
			desc = fmt.Sprintf("%s [env: %s]", desc, envVar)
		}

		fv := v.Field(i)
		if field.Type == reflect.TypeFor[time.Duration]() {
			p := addr[time.Duration](fv)
			cmd.Flags().DurationVar(p, flagName, *p, desc)
			continue
		}
		switch field.Type.Kind() {
		case reflect.String:
			p := addr[string](fv)
			cmd.Flags().StringVar(p, flagName, *p, desc)
		case reflect.Uint8:
			p := addr[uint8](fv)
			cmd.Flags().Uint8Var(p, flagName, *p, desc)
		case reflect.Uint16:
			p := addr[uint16](fv)
			cmd.Flags().Uint16Var(p, flagName, *p, desc)
		case reflect.Uint32:
			p := addr[uint32](fv)
			cmd.Flags().Uint32Var(p, flagName, *p, desc)
		case reflect.Uint64:
			p := addr[uint64](fv)
			cmd.Flags().Uint64Var(p, flagName, *p, desc)
		case reflect.Uint:
			p := addr[uint](fv)
			cmd.Flags().UintVar(p, flagName, *p, desc)
		case reflect.Int:
			p := addr[int](fv)
			cmd.Flags().IntVar(p, flagName, *p, desc)
		case reflect.Int8:
			p := addr[int8](fv)
			cmd.Flags().Int8Var(p, flagName, *p, desc)
		case reflect.Int16:
			p := addr[int16](fv)
			cmd.Flags().Int16Var(p, flagName, *p, desc)
		case reflect.Int32:
			p := addr[int32](fv)
			cmd.Flags().Int32Var(p, flagName, *p, desc)
		case reflect.Int64:
			p := addr[int64](fv)
			cmd.Flags().Int64Var(p, flagName, *p, desc)
		case reflect.Float64:
			p := addr[float64](fv)
			cmd.Flags().Float64Var(p, flagName, *p, desc)
		case reflect.Bool:
			p := addr[bool](fv)
			cmd.Flags().BoolVar(p, flagName, *p, desc)
		case reflect.Slice:
			if field.Type.Elem() != reflect.TypeFor[string]() {
				panic(fmt.Sprintf("config.RegisterFlags: field %s: flag %q only supports []string slices", field.Name, flagName))
			}
			p := addr[[]string](fv)
			cmd.Flags().StringSliceVar(p, flagName, *p, desc)
		default:
			panic(fmt.Sprintf("config.RegisterFlags: field %s (%s) has flag %q but unsupported reflect.Kind %s — add a case or remove the flag tag",
				field.Name, field.Type.String(), flagName, field.Type.Kind()))
		}
	}
}

// addr returns a pointer to the field fv as a *T, T being the type of its
// kind: a named type converts, as Go converts *Port to *uint16.
func addr[T any](fv reflect.Value) *T {
	return fv.Addr().Convert(reflect.TypeFor[*T]()).Interface().(*T)
}
