package types

import "strings"

func (c *Services) UnmarshalYAML(unmarshal func(interface{}) error) error {
	serviceMap := map[string]ServiceConfig{}
	err := unmarshal(&serviceMap)
	if err != nil {
		return err
	}

	for name, svc := range serviceMap {
		svc.Name = name
		*c = append(*c, svc)
	}
	return nil
}

type _ServiceVolumeConfig ServiceVolumeConfig

func (svc *ServiceVolumeConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var str string
	err := unmarshal(&str)
	if err == nil {
		parts := strings.Split(str, `:`)
		switch len(parts) {
		default:
			fallthrough
		case 3:
			if strings.ToLower(parts[2]) == `ro` {
				svc.ReadOnly = true
			}
			fallthrough
		case 2:
			svc.Target = parts[1]
			fallthrough
		case 1:
			svc.Source = parts[0]
		case 0:
		}
		svc.Type = `volume`
		return nil
	}

	return unmarshal((*_ServiceVolumeConfig)(svc))
}

func (svc ServiceVolumeConfig) MarshalYAML() (interface{}, error) {
	if svc.Type == `volume` && svc.Consistency == `` && svc.Bind == nil && svc.Volume == nil && svc.Tmpfs == nil {
		// Use the string form
		s := svc.Source
		if svc.Target != `` {
			s += `:` + svc.Target
			if svc.ReadOnly {
				s += `:ro`
			}
		}
		return s, nil
	}
	return _ServiceVolumeConfig(svc), nil
}
