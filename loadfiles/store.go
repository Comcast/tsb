/*
Copyright 2018 Comcast Cable Communications Management, LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package loadfiles

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"reflect"
	"strings"

	yaml "gopkg.in/yaml.v3"
)

func Store(dir File, obj interface{}) error {
	v := reflect.ValueOf(obj)
	for v.Kind() == reflect.Interface || v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return errors.New(`Illegal object kind ` + v.Kind().String() + `; struct expected.`)
	}

	if !v.CanSet() {
		return errors.New(`Cannot address ` + v.Type().String() + `; object must be mutable.`)
	}

	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		filespec := t.Field(i).Tag.Get(`file`)
		if filespec == `` {
			continue
		}

		specparts := strings.SplitN(filespec, `,`, 2)
		if len(specparts) < 2 {
			continue
		}

		format, filename := specparts[0], specparts[1]
		file := dir.In(filename)
		err := func() error {
			member := v.Field(i).Addr().Interface()
			if format == `file` {
				/* Handle this case first, since it's the only format that doesn't require opening the file. */
				return Store(file, member)
			}

			f, err := file.Create()
			if err != nil {
				return errors.New(`Unable to open file ` + file.String() + `: ` + err.Error())
			}
			defer f.Close()

			switch format {
			case `yaml`:
				b, err := yaml.Marshal(member)
				if err != nil {
					return errors.New(`Unable to marshal yaml ` + err.Error())
				}
				_, err = f.Write(b)
				if err != nil {
					return errors.New(`Unable to write to file ` + err.Error())
				}
				return nil
			case `json`:
				return json.NewEncoder(f).Encode(member)
			case `xml`:
				return xml.NewEncoder(f).Encode(member)
			case `text`:
				var err error
				txt := v.Field(i)
				if txt.Kind() == reflect.String {
					_, err = f.Write([]byte(txt.Interface().(string)))
				} else if txt.Kind() == reflect.Slice && txt.Type().Elem().Kind() == reflect.Uint8 {
					_, err = f.Write(txt.Interface().([]byte))
				} else {
					return errors.New(`Unable to store text value of ` + txt.Type().String() + ` for member ` + t.Field(i).Name)
				}

				if err != nil {
					return errors.New(`Unable to write text value ` + err.Error())
				}
				return nil
			}
			return errors.New(`Unknown format: ` + format)
		}()
		if err != nil {
			return err
		}
	}
	return nil
}
