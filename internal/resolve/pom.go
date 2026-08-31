package resolve

import (
	"encoding/xml"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/shevalab/mvn-repo-cleaner/internal/model"
)

// rawPOM mirrors the subset of a Maven POM we need for resolution.
type rawPOM struct {
	XMLName              xml.Name                 `xml:"project"`
	GroupID              string                   `xml:"groupId"`
	ArtifactID           string                   `xml:"artifactId"`
	Version              string                   `xml:"version"`
	Packaging            string                   `xml:"packaging"`
	Parent               *rawParent               `xml:"parent"`
	Properties           *rawProperties           `xml:"properties"`
	Dependencies         []rawDep                 `xml:"dependencies>dependency"`
	DependencyManagement *rawDependencyManagement `xml:"dependencyManagement"`
}

// rawProperties captures arbitrary nested property elements as a map.
type rawProperties struct {
	inner map[string]string
}

// UnmarshalXML decodes <properties><name>value</name>...</properties>.
func (p *rawProperties) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	p.inner = map[string]string{}
	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			var val string
			if err := d.DecodeElement(&val, &t); err != nil {
				return err
			}
			p.inner[t.Name.Local] = val
		case xml.EndElement:
			if t.Name == start.Name {
				return nil
			}
		}
	}
}

type rawParent struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
}

type rawDep struct {
	GroupID    string         `xml:"groupId"`
	ArtifactID string         `xml:"artifactId"`
	Version    string         `xml:"version"`
	Scope      string         `xml:"scope"`
	Optional   string         `xml:"optional"`
	Exclusions []rawExclusion `xml:"exclusions>exclusion"`
}

type rawExclusion struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
}

type rawDependencyManagement struct {
	Dependencies []rawDep `xml:"dependencies>dependency"`
}

var propRe = regexp.MustCompile(`\$\{([^}]+)\}`)

// interp replaces ${key} occurrences using props (and env fallbacks).
func interp(s string, props map[string]string) string {
	return propRe.ReplaceAllStringFunc(s, func(m string) string {
		key := propRe.FindStringSubmatch(m)[1]
		if v, ok := props[key]; ok {
			return v
		}
		if strings.HasPrefix(key, "env.") {
			if v, ok := os.LookupEnv(key[4:]); ok {
				return v
			}
		}
		return m
	})
}

// ParsePOM reads and parses a pom.xml file into a model.POM.
func ParsePOM(path string) (*model.POM, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var raw rawPOM
	if err := xml.NewDecoder(f).Decode(&raw); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	props := map[string]string{}
	if raw.Properties != nil {
		for k, v := range raw.Properties.inner {
			props[k] = v
		}
	}

	p := &model.POM{
		ArtifactID: raw.ArtifactID,
		Packaging:  raw.Packaging,
		Properties: props,
	}
	if p.Packaging == "" {
		p.Packaging = "jar"
	}
	if raw.GroupID != "" {
		p.GroupID = interp(raw.GroupID, props)
	}
	if raw.Version != "" {
		p.Version = interp(raw.Version, props)
	}
	if raw.Parent != nil {
		p.Parent = &model.Parent{
			GroupID:    interp(raw.Parent.GroupID, props),
			ArtifactID: interp(raw.Parent.ArtifactID, props),
			Version:    interp(raw.Parent.Version, props),
		}
	}

	for _, d := range raw.Dependencies {
		dep := model.Dep{
			GroupID:    interp(d.GroupID, props),
			ArtifactID: interp(d.ArtifactID, props),
			Version:    interp(d.Version, props),
			Scope:      normalizeScope(d.Scope),
			Optional:   strings.EqualFold(d.Optional, "true"),
		}
		for _, e := range d.Exclusions {
			dep.Exclusions = append(dep.Exclusions, model.Exclusion{
				GroupID:    interp(e.GroupID, props),
				ArtifactID: interp(e.ArtifactID, props),
			})
		}
		p.Dependencies = append(p.Dependencies, dep)
	}

	if raw.DependencyManagement != nil {
		for _, d := range raw.DependencyManagement.Dependencies {
			p.DependencyManagement = append(p.DependencyManagement, model.ManagedDep{
				GroupID:    interp(d.GroupID, props),
				ArtifactID: interp(d.ArtifactID, props),
				Version:    interp(d.Version, props),
				Scope:      normalizeScope(d.Scope),
			})
		}
	}

	return p, nil
}

func normalizeScope(scope string) string {
	switch strings.ToLower(scope) {
	case "compile", "provided", "runtime", "test", "system", "import":
		return strings.ToLower(scope)
	}
	return "compile"
}
