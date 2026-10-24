package api_test

import (
	"os"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/mateusmacedo/marketplace/apps/backend/bff/app/api"
)

type document struct {
	Paths      map[string]map[string]operation `yaml:"paths"`
	Components struct {
		Parameters      map[string]parameter `yaml:"parameters"`
		SecuritySchemes map[string]struct {
			Type   string `yaml:"type"`
			Scheme string `yaml:"scheme"`
		} `yaml:"securitySchemes"`
	} `yaml:"components"`
}

type operation struct {
	OperationID string                `yaml:"operationId"`
	Parameters  []parameter           `yaml:"parameters"`
	Responses   map[string]yaml.Node  `yaml:"responses"`
	Security    []map[string][]string `yaml:"security"`
}

type parameter struct {
	Ref      string `yaml:"$ref"`
	Name     string `yaml:"name"`
	In       string `yaml:"in"`
	Required bool   `yaml:"required"`
}

func loadDocument(t *testing.T, file string) document {
	t.Helper()
	raw, err := os.ReadFile("../../../../../" + file)
	if err != nil {
		t.Fatalf("ReadFile(%s) = %v", file, err)
	}
	var doc document
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("%s is not YAML: %v", file, err)
	}
	return doc
}

func (d document) resolve(p parameter) parameter {
	if name, ok := strings.CutPrefix(p.Ref, "#/components/parameters/"); ok {
		return d.Components.Parameters[name]
	}
	return p
}

func TestTheContractDeclaresEveryRouteTheEdgeServes(t *testing.T) {
	// WHY: every refusal the edge answers for an authenticated route is in the
	// contract, so a generated client reads each one as Rejection.
	statuses := map[string][]string{
		"me": {"200", "401", "403", "429", "default"},
	}
	routes := api.Routes(routeBudget)
	if len(routes) == 0 {
		t.Fatal("Routes() is empty, want every route the edge serves checked against the contract")
	}
	for _, route := range routes {
		t.Run(route.Name, func(t *testing.T) {
			file, pointer, found := strings.Cut(route.ContractRef, "#/paths/")
			if !found {
				t.Fatalf("ContractRef %q is not a pointer into paths", route.ContractRef)
			}
			escaped, method, _ := strings.Cut(pointer, "/")
			path := strings.ReplaceAll(escaped, "~1", "/")
			if path != route.Path || method != strings.ToLower(route.Method) {
				t.Fatalf("ContractRef names %s %s, want %s %s", method, path, route.Method, route.Path)
			}

			doc := loadDocument(t, file)
			op, declared := doc.Paths[path][method]
			if !declared {
				t.Fatalf("%s does not declare %s %s", file, method, path)
			}
			if op.OperationID != route.Name {
				t.Fatalf("%s operationId = %q, want the route name %q", path, op.OperationID, route.Name)
			}
			if bearer := doc.Components.SecuritySchemes["bearerAuth"]; bearer.Type != "http" || bearer.Scheme != "bearer" {
				t.Fatalf("bearerAuth = %+v, want an http bearer scheme", bearer)
			}
			if len(op.Security) != 1 || op.Security[0]["bearerAuth"] == nil {
				t.Fatalf("%s security = %v, want bearerAuth", route.Name, op.Security)
			}

			byName := map[string]parameter{}
			for _, p := range op.Parameters {
				resolved := doc.resolve(p)
				byName[resolved.Name] = resolved
			}
			if correlation, ok := byName[api.CorrelationHeader]; !ok || correlation.In != "header" || correlation.Required {
				t.Fatalf("%s: %s must be an optional header, got %+v", route.Name, api.CorrelationHeader, correlation)
			}

			var got []string
			for code := range op.Responses {
				got = append(got, code)
			}
			slices.Sort(got)
			want := slices.Clone(statuses[route.Name])
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Fatalf("%s responses = %v, want %v", route.Name, got, want)
			}
		})
	}
}
