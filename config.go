package main

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

type CommandConfig struct {
}

func (x *CommandConfig) showValue(s interface{}, name string, indent string, pad string) error {
	fmt.Printf("%s\033[34m%s\033[0m:%s ", indent, name, pad)

	switch v := s.(type) {
	case string:
		fmt.Printf("\033[32m%s\033[0m\n", v)
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		fmt.Printf("\033[36m%v\033[0m\n", v)
	default:
		fmt.Printf("%v\n", s)
	}

	return nil
}

func (x *CommandConfig) fieldName(f reflect.StructField) string {
	js := f.Tag.Get("json")

	if len(js) != 0 && !strings.HasPrefix(js, "-") && f.Tag.Get("config") != "-" {
		return strings.Split(js, ",")[0]
	}

	return ""
}

func (x *CommandConfig) show(s interface{}, name string, indent string, pad string) error {
	v := reflect.ValueOf(s)

	if v.Kind() == reflect.Ptr && !v.IsNil() {
		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.Struct:
		tp := v.Type()

		fields := make([]int, 0, tp.NumField())
		fieldnames := make([]string, 0, tp.NumField())
		longest := 0

		for i := 0; i < tp.NumField(); i++ {
			field := tp.Field(i)
			fname := x.fieldName(field)

			if len(fname) != 0 {
				fields = append(fields, i)
				fieldnames = append(fieldnames, fname)

				if len(fname) > longest {
					longest = len(fname)
				}
			}
		}

		if len(fields) > 0 && len(name) > 0 || len(indent) > 0 {
			fmt.Println()

			if len(name) > 0 {
				fmt.Printf("%s\033[34;1m%s\033[0m:\n", indent, name)
			} else {
				fmt.Printf(indent + pad + " ")
			}
		}

		for i, idx := range fields {
			var nn string
			var nindent string

			fname := fieldnames[i]

			if len(name) == 0 {
				nn = fname
				nindent = ""
			} else {
				nn = "." + fname
				nindent = indent + "  "
			}

			vv := v.Field(idx)
			p := strings.Repeat(" ", longest-len(fname))

			if err := x.show(vv.Interface(), nn, nindent, p); err != nil {
				return err
			}
		}

		if len(fields) > 0 && len(name) > 0 {
			fmt.Println()
		}
	case reflect.Array, reflect.Slice, reflect.Map:
	default:
		if err := x.showValue(s, name, indent, pad); err != nil {
			return err
		}
	}

	return nil
}

func (x *CommandConfig) findField(s reflect.Value, field string) (*reflect.StructField, *reflect.Value) {
	if s.Kind() == reflect.Ptr && !s.IsNil() {
		s = s.Elem()
	}

	if s.Kind() != reflect.Struct {
		return nil, nil
	}

	fields := strings.SplitN(field, ".", 2)
	fieldname := strings.ToLower(fields[0])
	tp := s.Type()
	fidx := -1

	var fld reflect.StructField

	for i := 0; i < tp.NumField(); i++ {
		fld = tp.Field(i)
		fldname := x.fieldName(fld)

		if len(fldname) > 0 && strings.ToLower(fldname) == fieldname {
			fidx = i
			break
		}
	}

	if fidx == -1 {
		return nil, nil
	}

	fldval := s.Field(fidx)

	if len(fields) == 1 {
		return &fld, &fldval
	} else if fldval.Kind() == reflect.Struct {
		return x.findField(fldval, fields[1])
	} else if fldval.Kind() == reflect.Ptr && fldval.Elem().Kind() == reflect.Struct && !fldval.IsNil() {
		return x.findField(fldval.Elem(), fields[1])
	}

	return nil, nil
}

func (x *CommandConfig) assign(v reflect.Value, val string) error {
	switch v.Kind() {
	case reflect.String:
		v.SetString(val)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		vint, err := strconv.ParseInt(val, 10, 64)

		if err != nil {
			return err
		}

		v.SetInt(vint)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		vint, err := strconv.ParseUint(val, 10, 64)

		if err != nil {
			return err
		}

		v.SetUint(vint)
	case reflect.Float32, reflect.Float64:
		vflt, err := strconv.ParseFloat(val, 64)

		if err != nil {
			return err
		}

		v.SetFloat(vflt)
	case reflect.Bool:
		vbl, err := strconv.ParseBool(val)

		if err != nil {
			return err
		}

		v.SetBool(vbl)
	default:
		return fmt.Errorf("Failed to set value of type %v", v.Kind())
	}

	return nil
}

func (x *CommandConfig) Execute(args []string) error {
	if len(args) == 0 {
		return x.show(options, "", "", "")
	}

	shown := false

	err := options.UpdateConfig(func(opts *Options) error {
		opt := reflect.ValueOf(options)

		for _, arg := range args {
			parts := strings.SplitN(arg, "=", 2)
			fld, val := x.findField(opt, parts[0])

			if fld == nil {
				return fmt.Errorf("Could not find configuration `%s'", parts[0])
			}

			if len(parts) == 1 {
				desc := fld.Tag.Get("description")

				if len(desc) > 0 {
					fmt.Printf("\n\033[35m# %s\033[0m\n", desc)
				}

				x.show(val.Interface(), arg, "", "")
				shown = true
			} else {
				x.assign(*val, parts[1])
			}
		}

		return nil
	})

	if err != nil {
		return err
	}

	if shown {
		fmt.Println()
	}

	return nil
}

func init() {
	parser.AddCommand("config",
		"Get or set configuration settings",
		"The config command can be used to get or set configuration settings. Without any arguments, all the current configuration settings are listed. You can list specific values of configuration settings by providing one or more setting names as arguments. Finally, to set a configuration value, use `setting=value` as an argument.",
		&CommandConfig{})
}
