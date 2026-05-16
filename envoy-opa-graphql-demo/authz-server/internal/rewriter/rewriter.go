package rewriter

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/formatter"
	"github.com/vektah/gqlparser/v2/parser"
)

// ErrEmptyQuery is returned when all requested fields are denied and the query becomes empty.
var ErrEmptyQuery = errors.New("all requested fields are denied")

type deniedFieldSet struct {
	global map[string]struct{}
	paths  map[string]struct{}
}

func parseDeniedFields(deniedFields []string) deniedFieldSet {
	ds := deniedFieldSet{
		global: make(map[string]struct{}),
		paths:  make(map[string]struct{}),
	}
	for _, f := range deniedFields {
		if strings.Contains(f, ".") {
			ds.paths[f] = struct{}{}
		} else {
			ds.global[f] = struct{}{}
		}
	}
	return ds
}

func (ds *deniedFieldSet) isDenied(currentPath, fieldName string) bool {
	if _, ok := ds.global[fieldName]; ok {
		return true
	}
	if len(ds.paths) == 0 {
		return false
	}
	var fullPath string
	if currentPath == "" {
		fullPath = fieldName
	} else {
		fullPath = currentPath + "." + fieldName
	}
	_, ok := ds.paths[fullPath]
	return ok
}

// RewriteBody 解析 HTTP body (JSON)，过滤 GraphQL query 中的指定字段，返回改写后的 body。
// 支持单个 JSON 对象和 JSON 数组（batch）格式。
// deniedFields 中的字段名将从 query 的所有 SelectionSet 中移除。
func RewriteBody(body []byte, deniedFields []string) ([]byte, error) {
	if len(deniedFields) == 0 {
		return body, nil
	}

	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		return rewriteBatch(body, deniedFields)
	}
	return rewriteSingle(body, deniedFields)
}

func rewriteSingle(body []byte, deniedFields []string) ([]byte, error) {
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal body: %w", err)
	}

	queryRaw, ok := payload["query"]
	if !ok {
		return body, nil
	}
	query, ok := queryRaw.(string)
	if !ok {
		return body, nil
	}

	rewritten, err := removeFields(query, deniedFields)
	if err != nil {
		return nil, err
	}

	payload["query"] = rewritten
	return json.Marshal(payload)
}

func rewriteBatch(body []byte, deniedFields []string) ([]byte, error) {
	var batch []map[string]interface{}
	if err := json.Unmarshal(body, &batch); err != nil {
		return nil, fmt.Errorf("unmarshal batch body: %w", err)
	}

	for i, payload := range batch {
		queryRaw, ok := payload["query"]
		if !ok {
			continue
		}
		query, ok := queryRaw.(string)
		if !ok {
			continue
		}
		rewritten, err := removeFields(query, deniedFields)
		if err != nil {
			return nil, fmt.Errorf("batch[%d]: %w", i, err)
		}
		payload["query"] = rewritten
	}

	return json.Marshal(batch)
}

// removeFields 从 GraphQL query 字符串中移除指定字段。
func removeFields(query string, deniedFields []string) (string, error) {
	doc, err := parser.ParseQuery(&ast.Source{Input: query})
	if err != nil {
		return "", fmt.Errorf("parse query: %w", err)
	}

	ds := parseDeniedFields(deniedFields)

	// Pass 1: filter fragment definitions, collect empty ones.
	// Fragments have no path context — only global rules apply.
	emptyFragments := make(map[string]struct{})
	remaining := make(ast.FragmentDefinitionList, 0, len(doc.Fragments))
	for _, frag := range doc.Fragments {
		frag.SelectionSet = filterSelectionSet(frag.SelectionSet, &ds, "")
		if len(frag.SelectionSet) == 0 {
			emptyFragments[frag.Name] = struct{}{}
		} else {
			remaining = append(remaining, frag)
		}
	}
	doc.Fragments = remaining

	// Pass 2: filter operations.
	for _, op := range doc.Operations {
		op.SelectionSet = filterSelectionSet(op.SelectionSet, &ds, "")
		if len(op.SelectionSet) == 0 {
			return "", ErrEmptyQuery
		}
	}

	// Pass 3: remove spreads referencing empty fragments.
	if len(emptyFragments) > 0 {
		for _, op := range doc.Operations {
			op.SelectionSet = removeEmptyFragmentSpreads(op.SelectionSet, emptyFragments)
			if len(op.SelectionSet) == 0 {
				return "", ErrEmptyQuery
			}
		}
		for _, frag := range doc.Fragments {
			frag.SelectionSet = removeEmptyFragmentSpreads(frag.SelectionSet, emptyFragments)
		}
	}

	var buf strings.Builder
	formatter.NewFormatter(&buf).FormatQueryDocument(doc)
	return strings.TrimSpace(buf.String()), nil
}

func filterSelectionSet(set ast.SelectionSet, ds *deniedFieldSet, currentPath string) ast.SelectionSet {
	if len(set) == 0 {
		return set
	}

	filtered := make(ast.SelectionSet, 0, len(set))
	for _, sel := range set {
		switch node := sel.(type) {
		case *ast.Field:
			if ds.isDenied(currentPath, node.Name) {
				continue
			}
			if len(node.SelectionSet) > 0 {
				var childPath string
				if currentPath == "" {
					childPath = node.Name
				} else {
					childPath = currentPath + "." + node.Name
				}
				node.SelectionSet = filterSelectionSet(node.SelectionSet, ds, childPath)
				if len(node.SelectionSet) == 0 {
					continue
				}
			}
			filtered = append(filtered, node)
		case *ast.InlineFragment:
			node.SelectionSet = filterSelectionSet(node.SelectionSet, ds, currentPath)
			if len(node.SelectionSet) > 0 {
				filtered = append(filtered, node)
			}
		case *ast.FragmentSpread:
			filtered = append(filtered, node)
		default:
			filtered = append(filtered, node)
		}
	}
	return filtered
}

func removeEmptyFragmentSpreads(set ast.SelectionSet, emptyFrags map[string]struct{}) ast.SelectionSet {
	if len(set) == 0 {
		return set
	}
	filtered := make(ast.SelectionSet, 0, len(set))
	for _, sel := range set {
		switch node := sel.(type) {
		case *ast.FragmentSpread:
			if _, empty := emptyFrags[node.Name]; empty {
				continue
			}
			filtered = append(filtered, node)
		case *ast.Field:
			if len(node.SelectionSet) > 0 {
				node.SelectionSet = removeEmptyFragmentSpreads(node.SelectionSet, emptyFrags)
			}
			filtered = append(filtered, node)
		case *ast.InlineFragment:
			node.SelectionSet = removeEmptyFragmentSpreads(node.SelectionSet, emptyFrags)
			filtered = append(filtered, node)
		default:
			filtered = append(filtered, node)
		}
	}
	return filtered
}
