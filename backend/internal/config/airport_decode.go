package config

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dlclark/regexp2"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"go.yaml.in/yaml/v4"
)

// Keep the shipped contract byte-identical to the reviewed authoring schema.
//
//go:embed airport-configuration.schema.json
var airportSchema []byte
var compiledAirportSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	var value any
	if err := json.Unmarshal(airportSchema, &value); err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	compiler.UseRegexpEngine(func(pattern string) (jsonschema.Regexp, error) {
		re, err := regexp2.Compile(pattern, regexp2.ECMAScript)
		if err != nil {
			return nil, err
		}
		re.MatchTimeout = time.Second
		return schemaRegexp{re}, nil
	})
	if err := compiler.AddResource("airport.json", value); err != nil {
		return nil, err
	}
	return compiler.Compile("airport.json")
})

type schemaRegexp struct{ *regexp2.Regexp }

func (r schemaRegexp) MatchString(value string) bool {
	ok, err := r.Regexp.MatchString(value)
	return err == nil && ok
}

// DecodeAirport validates the complete structural and domain contract before
// returning concrete configuration. It has no side effects on loaded airports.
func DecodeAirport(source string, r io.Reader) (result *AirportDocument, err error) {
	airport := "<unknown>"
	defer func() {
		if err != nil {
			err = fmt.Errorf("%s airport %s: %w", source, airport, err)
		}
	}()
	decoder := yaml.NewDecoder(r)
	var root yaml.Node
	if err = decoder.Decode(&root); err != nil {
		return nil, err
	}
	if len(root.Content) != 1 {
		return nil, errors.New("expected one airport document")
	}
	var extra yaml.Node
	if trailing := decoder.Decode(&extra); trailing != io.EOF {
		return nil, fmt.Errorf("expected a single YAML document, got trailing document or error: %v", trailing)
	}
	node := root.Content[0]
	if name := mappingValue(node, "airport"); name != nil {
		airport = name.Value
	}
	value, err := airportJSON(node, "$")
	if err != nil {
		return nil, err
	}
	schema, err := compiledAirportSchema()
	if err != nil {
		return nil, err
	}
	if err = schema.Validate(value); err != nil {
		line, column := node.Line, node.Column
		var validation *jsonschema.ValidationError
		if errors.As(err, &validation) {
			location := deepestValidation(validation)
			if n := airportNodeAt(node, location.InstanceLocation); n != nil {
				line, column = n.Line, n.Column
			}
		}
		return nil, fmt.Errorf("line %d column %d: %w", line, column, err)
	}
	// Domain scalar readers must receive the values that passed validation,
	// rather than interpreting YAML octal/hex/exponent spellings as text.
	canonicalizeAirportNumbers(node)
	encoded, err := yaml.Marshal(node)
	if err != nil {
		return nil, err
	}
	var doc AirportDocument
	typed := yaml.NewDecoder(bytes.NewReader(encoded))
	typed.KnownFields(true)
	if err = typed.Decode(&doc); err != nil {
		return nil, err
	}
	normalizeAirportRunwaySelectors(&doc)
	settings := mappingValue(mappingValue(node, "workflow"), "settings")
	switch doc.Workflow.Handler {
	case "ekch/v1":
		var cfg EKCHSettings
		if err = settings.Decode(&cfg); err != nil {
			return nil, err
		}
		doc.Workflow.Settings = cfg
	default:
		return nil, fmt.Errorf("workflow.handler: no compiled settings for %q", doc.Workflow.Handler)
	}
	if err = validateAirportDomains(&doc); err != nil {
		return nil, fmt.Errorf("line %d: %w", node.Line, err)
	}
	return &doc, nil
}

func canonicalizeAirportNumbers(node *yaml.Node) {
	if node.Kind == yaml.ScalarNode && (node.Tag == "!!int" || node.Tag == "!!float") {
		var value any
		// airportJSON already decoded and validated every scalar.
		_ = node.Decode(&value)
		switch number := value.(type) {
		case int:
			node.Value = strconv.Itoa(number)
		case uint64:
			node.Value = strconv.FormatUint(number, 10)
		case float64:
			if math.Trunc(number) == number {
				node.Tag = "!!int"
				node.Value = strconv.FormatFloat(number, 'f', -1, 64)
			} else {
				node.Value = strconv.FormatFloat(number, 'g', -1, 64)
			}
		}
	}
	for _, child := range node.Content {
		canonicalizeAirportNumbers(child)
	}
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}
func deepestValidation(err *jsonschema.ValidationError) *jsonschema.ValidationError {
	result := err
	for _, cause := range err.Causes {
		candidate := deepestValidation(cause)
		if len(candidate.InstanceLocation) > len(result.InstanceLocation) {
			result = candidate
		}
	}
	return result
}
func airportNodeAt(node *yaml.Node, path []string) *yaml.Node {
	for _, key := range path {
		if node == nil {
			return nil
		}
		if node.Kind == yaml.MappingNode {
			node = mappingValue(node, key)
		} else if node.Kind == yaml.SequenceNode {
			i, err := strconv.Atoi(key)
			if err != nil || i < 0 || i >= len(node.Content) {
				return node
			}
			node = node.Content[i]
		} else {
			return node
		}
	}
	return node
}

// Preserve YAML scalar types for schema validation; reject duplicate keys,
// aliases/merges and non-string mapping keys instead of coercing them silently.
func airportJSON(node *yaml.Node, path string) (any, error) {
	fail := func(message string) (any, error) {
		return nil, fmt.Errorf("%s line %d column %d: %s", path, node.Line, node.Column, message)
	}
	switch node.Kind {
	case yaml.MappingNode:
		result := map[string]any{}
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			if key.Tag != "!!str" {
				return fail("mapping keys must be strings")
			}
			if _, exists := result[key.Value]; exists {
				return fail("duplicate YAML key " + key.Value)
			}
			value, err := airportJSON(node.Content[i+1], path+"."+key.Value)
			if err != nil {
				return nil, err
			}
			result[key.Value] = value
		}
		if strings.Contains(path, ".forStandRanges[") {
			from, to := airportNumber(result["from"]), airportNumber(result["to"])
			if from != nil && to != nil && from.Cmp(to) > 0 {
				return fail("stand range is inverted")
			}
		}
		return result, nil
	case yaml.SequenceNode:
		result := make([]any, len(node.Content))
		for i, child := range node.Content {
			value, err := airportJSON(child, fmt.Sprintf("%s[%d]", path, i))
			if err != nil {
				return nil, err
			}
			result[i] = value
		}
		return result, nil
	case yaml.ScalarNode:
		switch node.Tag {
		case "!!str", "!!bool", "!!int", "!!float", "!!null":
			var value any
			if err := node.Decode(&value); err != nil {
				return nil, err
			}
			if number, ok := value.(float64); ok && (math.IsNaN(number) || math.IsInf(number, 0)) {
				return fail("numbers must be finite")
			}
			return value, nil
		}
	}
	return fail("unsupported YAML node/tag (aliases and merges are not accepted)")
}

func airportNumber(value any) *big.Rat {
	switch number := value.(type) {
	case int:
		return new(big.Rat).SetInt64(int64(number))
	case int64:
		return new(big.Rat).SetInt64(number)
	case uint64:
		return new(big.Rat).SetInt(new(big.Int).SetUint64(number))
	case float64:
		return new(big.Rat).SetFloat64(number)
	default:
		return nil
	}
}
