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
	Build                *rawBuild                `xml:"build"`
	Profiles             *rawProfiles             `xml:"profiles"`
}

type rawBuild struct {
	Plugins          []rawPlugin          `xml:"plugins>plugin"`
	PluginManagement *rawPluginManagement `xml:"pluginManagement"`
}

type rawPluginManagement struct {
	Plugins []rawPlugin `xml:"plugins>plugin"`
}

type rawPlugin struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
	// Configuration captures the plugin <configuration> block so artifactItem
	// references (used by maven-dependency-plugin) can be extracted.
	Configuration *rawConfiguration `xml:"configuration"`
}

type rawConfiguration struct {
	ArtifactItems []artifactItem `xml:"artifactItems>artifactItem"`
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

type rawProfile struct {
	Properties           *rawProperties           `xml:"properties"`
	Dependencies         []rawDep                 `xml:"dependencies>dependency"`
	DependencyManagement *rawDependencyManagement `xml:"dependencyManagement"`
	Plugins              []rawPlugin              `xml:"build>plugins>plugin"`
}

type rawProfiles struct {
	Profiles []rawProfile `xml:"profile"`
}

// artifactItem is a coordinate referenced inside a plugin's
// <configuration><artifactItems><artifactItem>. Only groupId/artifactId/version
// are needed for in-use detection.
type artifactItem struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
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

// defaultPluginGroup is the Maven default groupId for build plugins declared
// without an explicit <groupId>.
const defaultPluginGroup = "org.apache.maven.plugins"

// plugin from a raw plugin element, defaulting the groupId to Maven's standard
// plugin group when omitted.
func pluginFrom(raw rawPlugin, props map[string]string) model.Plugin {
	g := interp(raw.GroupID, props)
	if g == "" {
		g = defaultPluginGroup
	}
	p := model.Plugin{
		GroupID:    g,
		ArtifactID: interp(raw.ArtifactID, props),
		Version:    interp(raw.Version, props),
	}
	if raw.Configuration != nil {
		for _, item := range raw.Configuration.ArtifactItems {
			gid := interp(item.GroupID, props)
			aid := interp(item.ArtifactID, props)
			if gid == "" || aid == "" {
				continue
			}
			p.ArtifactItems = append(p.ArtifactItems, model.Coordinate{
				Artifact: model.Artifact{GroupID: gid, ArtifactID: aid},
				Version:  interp(item.Version, props),
			})
		}
	}
	return p
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
		p.Dependencies = append(p.Dependencies, depFrom(d, props))
	}

	if raw.DependencyManagement != nil {
		for _, d := range raw.DependencyManagement.Dependencies {
			p.DependencyManagement = append(p.DependencyManagement, managedFrom(d, props))
		}
	}

	if raw.Build != nil {
		for _, pl := range raw.Build.Plugins {
			p.Plugins = append(p.Plugins, pluginFrom(pl, props))
		}
		if raw.Build.PluginManagement != nil {
			for _, pl := range raw.Build.PluginManagement.Plugins {
				p.PluginManagement = append(p.PluginManagement, pluginFrom(pl, props))
			}
		}
	}

	if raw.Profiles != nil {
		for _, rp := range raw.Profiles.Profiles {
			p.Profiles = append(p.Profiles, profileFrom(rp, props))
		}
	}

	return p, nil
}

// depFrom converts a raw dependency to a model.Dep with property interpolation.
func depFrom(d rawDep, props map[string]string) model.Dep {
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
	return dep
}

// managedFrom converts a raw managed dependency with property interpolation.
func managedFrom(d rawDep, props map[string]string) model.ManagedDep {
	return model.ManagedDep{
		GroupID:    interp(d.GroupID, props),
		ArtifactID: interp(d.ArtifactID, props),
		Version:    interp(d.Version, props),
		Scope:      normalizeScope(d.Scope),
	}
}

// profileFrom converts a raw profile (dependencies, dependencyManagement,
// plugins and properties), merging profile-local properties for interpolation.
func profileFrom(rp rawProfile, inherited map[string]string) model.Profile {
	props := map[string]string{}
	for k, v := range inherited {
		props[k] = v
	}
	if rp.Properties != nil {
		for k, v := range rp.Properties.inner {
			props[k] = v
		}
	}
	prof := model.Profile{Properties: props}
	for _, d := range rp.Dependencies {
		prof.Dependencies = append(prof.Dependencies, depFrom(d, props))
	}
	if rp.DependencyManagement != nil {
		for _, d := range rp.DependencyManagement.Dependencies {
			prof.DependencyManagement = append(prof.DependencyManagement, managedFrom(d, props))
		}
	}
	for _, pl := range rp.Plugins {
		prof.Plugins = append(prof.Plugins, pluginFrom(pl, props))
	}
	return prof
}

func normalizeScope(scope string) string {
	switch strings.ToLower(scope) {
	case "compile", "provided", "runtime", "test", "system", "import":
		return strings.ToLower(scope)
	}
	return "compile"
}
