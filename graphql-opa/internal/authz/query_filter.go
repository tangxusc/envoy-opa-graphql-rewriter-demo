package authz

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/formatter"
	"github.com/vektah/gqlparser/v2/parser"
)

const removedEmployeeSalaryField = "employeeByID.salary"

type salaryReadChecker func(ctx context.Context, user User, targetID string) (bool, error)

type filterContext struct {
	inEmployeeByID bool
	salaryAllowed  bool
}

type fragmentVariantKey struct {
	name           string
	inEmployeeByID bool
	salaryAllowed  bool
}

type queryFilter struct {
	ctx           context.Context
	doc           *ast.QueryDocument
	user          User
	variables     map[string]any
	canReadSalary salaryReadChecker

	originalFragments map[string]*ast.FragmentDefinition
	currentFragments  map[string]*ast.FragmentDefinition
	variantByKey      map[fragmentVariantKey]string
	usedFragmentNames map[string]struct{}
	removedFieldsSet  map[string]struct{}
}

func filterGraphQLQuery(
	ctx context.Context,
	query string,
	user User,
	variables map[string]any,
	canReadSalary salaryReadChecker,
) (string, []string, bool, error) {
	doc, err := parser.ParseQuery(&ast.Source{Input: query})
	if err != nil {
		return "", nil, false, fmt.Errorf("parse query: %w", err)
	}

	filter := newQueryFilter(ctx, doc, user, variables, canReadSalary)
	if err := filter.filterDocument(); err != nil {
		return "", nil, false, err
	}

	removedFields := filter.removedFields()
	if len(doc.Operations) == 0 {
		return "", removedFields, false, nil
	}

	var builder strings.Builder
	formatter.NewFormatter(&builder).FormatQueryDocument(doc)
	filteredQuery := strings.TrimSpace(builder.String())
	if filteredQuery == "" {
		return "", removedFields, false, nil
	}

	if _, err := parser.ParseQuery(&ast.Source{Input: filteredQuery}); err != nil {
		return "", nil, false, fmt.Errorf("validate filtered query: %w", err)
	}

	return filteredQuery, removedFields, true, nil
}

func newQueryFilter(
	ctx context.Context,
	doc *ast.QueryDocument,
	user User,
	variables map[string]any,
	canReadSalary salaryReadChecker,
) *queryFilter {
	safeVariables := variables
	if safeVariables == nil {
		safeVariables = map[string]any{}
	}

	originalFragments := make(map[string]*ast.FragmentDefinition, len(doc.Fragments))
	for _, fragment := range doc.Fragments {
		originalFragments[fragment.Name] = fragment
	}

	return &queryFilter{
		ctx:               ctx,
		doc:               doc,
		user:              user,
		variables:         safeVariables,
		canReadSalary:     canReadSalary,
		originalFragments: originalFragments,
		currentFragments:  map[string]*ast.FragmentDefinition{},
		variantByKey:      map[fragmentVariantKey]string{},
		usedFragmentNames: map[string]struct{}{},
		removedFieldsSet:  map[string]struct{}{},
	}
}

func (f *queryFilter) filterDocument() error {
	f.doc.Fragments = ast.FragmentDefinitionList{}

	filteredOperations := make(ast.OperationList, 0, len(f.doc.Operations))
	for _, operation := range f.doc.Operations {
		filteredSelectionSet, err := f.filterSelectionSet(operation.SelectionSet, filterContext{})
		if err != nil {
			return err
		}
		operation.SelectionSet = filteredSelectionSet
		if len(operation.SelectionSet) == 0 {
			continue
		}
		filteredOperations = append(filteredOperations, operation)
	}
	f.doc.Operations = filteredOperations

	f.pruneUnusedFragments()
	return nil
}

func (f *queryFilter) filterSelectionSet(selectionSet ast.SelectionSet, ctx filterContext) (ast.SelectionSet, error) {
	filtered := make(ast.SelectionSet, 0, len(selectionSet))
	for _, selection := range selectionSet {
		switch node := selection.(type) {
		case *ast.Field:
			if ctx.inEmployeeByID && node.Name == "salary" && !ctx.salaryAllowed {
				f.removedFieldsSet[removedEmployeeSalaryField] = struct{}{}
				continue
			}

			childCtx := ctx
			if node.Name == "employeeByID" {
				allowed, err := f.canReadSalaryForField(node)
				if err != nil {
					return nil, err
				}
				childCtx = filterContext{
					inEmployeeByID: true,
					salaryAllowed:  allowed,
				}
			}

			if len(node.SelectionSet) > 0 {
				childSelectionSet, err := f.filterSelectionSet(node.SelectionSet, childCtx)
				if err != nil {
					return nil, err
				}
				node.SelectionSet = childSelectionSet
				if len(node.SelectionSet) == 0 {
					continue
				}
			}

			filtered = append(filtered, node)
		case *ast.InlineFragment:
			childSelectionSet, err := f.filterSelectionSet(node.SelectionSet, ctx)
			if err != nil {
				return nil, err
			}
			node.SelectionSet = childSelectionSet
			if len(node.SelectionSet) == 0 {
				continue
			}
			filtered = append(filtered, node)
		case *ast.FragmentSpread:
			fragmentName, keep, err := f.ensureFragmentVariant(node.Name, ctx)
			if err != nil {
				return nil, err
			}
			if !keep {
				continue
			}

			clonedSpread := *node
			clonedSpread.Name = fragmentName
			filtered = append(filtered, &clonedSpread)
		default:
			filtered = append(filtered, node)
		}
	}
	return filtered, nil
}

func (f *queryFilter) ensureFragmentVariant(name string, ctx filterContext) (string, bool, error) {
	original, exists := f.originalFragments[name]
	if !exists {
		return name, true, nil
	}

	key := fragmentVariantKey{
		name:           name,
		inEmployeeByID: ctx.inEmployeeByID,
		salaryAllowed:  ctx.salaryAllowed,
	}
	if variantName, ok := f.variantByKey[key]; ok {
		fragment := f.currentFragments[variantName]
		return variantName, fragment != nil && len(fragment.SelectionSet) > 0, nil
	}

	variantName := f.generateFragmentName(name, ctx)
	cloned := cloneFragmentDefinition(original)
	cloned.Name = variantName

	f.variantByKey[key] = variantName
	f.currentFragments[variantName] = cloned
	f.doc.Fragments = append(f.doc.Fragments, cloned)

	filteredSelectionSet, err := f.filterSelectionSet(cloned.SelectionSet, ctx)
	if err != nil {
		return "", false, err
	}
	cloned.SelectionSet = filteredSelectionSet

	if len(cloned.SelectionSet) == 0 {
		return variantName, false, nil
	}
	return variantName, true, nil
}

func (f *queryFilter) generateFragmentName(base string, ctx filterContext) string {
	if !ctx.inEmployeeByID && f.reserveFragmentName(base) {
		return base
	}

	suffix := "deny"
	if ctx.salaryAllowed {
		suffix = "allow"
	}
	candidateBase := fmt.Sprintf("%s__authz_%s", base, suffix)
	for i := 0; ; i++ {
		candidate := candidateBase
		if i > 0 {
			candidate = fmt.Sprintf("%s_%d", candidateBase, i)
		}
		if f.reserveFragmentName(candidate) {
			return candidate
		}
	}
}

func (f *queryFilter) reserveFragmentName(name string) bool {
	if _, exists := f.usedFragmentNames[name]; exists {
		return false
	}
	f.usedFragmentNames[name] = struct{}{}
	return true
}

func (f *queryFilter) canReadSalaryForField(field *ast.Field) (bool, error) {
	targetID := resolveTargetID(field.Arguments.ForName("id"), f.variables)
	allowed, err := f.canReadSalary(f.ctx, f.user, targetID)
	if err != nil {
		return false, fmt.Errorf("check salary permission for target %q: %w", targetID, err)
	}
	return allowed, nil
}

func resolveTargetID(argument *ast.Argument, variables map[string]any) string {
	if argument == nil || argument.Value == nil {
		return ""
	}

	value, err := argument.Value.Value(variables)
	if err != nil || value == nil {
		return ""
	}

	targetID, ok := value.(string)
	if !ok {
		return ""
	}
	return targetID
}

func (f *queryFilter) pruneUnusedFragments() {
	if len(f.doc.Fragments) == 0 {
		return
	}

	used := map[string]struct{}{}
	queue := make([]string, 0)

	enqueue := func(name string) {
		if name == "" {
			return
		}
		if _, exists := used[name]; exists {
			return
		}
		used[name] = struct{}{}
		queue = append(queue, name)
	}

	for _, operation := range f.doc.Operations {
		for _, name := range collectFragmentSpreads(operation.SelectionSet) {
			enqueue(name)
		}
	}

	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]

		fragment := f.currentFragments[name]
		if fragment == nil || len(fragment.SelectionSet) == 0 {
			continue
		}
		for _, nested := range collectFragmentSpreads(fragment.SelectionSet) {
			enqueue(nested)
		}
	}

	filteredFragments := make(ast.FragmentDefinitionList, 0, len(used))
	filteredFragmentMap := make(map[string]*ast.FragmentDefinition, len(used))
	for _, fragment := range f.doc.Fragments {
		if _, exists := used[fragment.Name]; !exists {
			continue
		}
		if len(fragment.SelectionSet) == 0 {
			continue
		}
		filteredFragments = append(filteredFragments, fragment)
		filteredFragmentMap[fragment.Name] = fragment
	}

	f.doc.Fragments = filteredFragments
	f.currentFragments = filteredFragmentMap
}

func collectFragmentSpreads(selectionSet ast.SelectionSet) []string {
	names := map[string]struct{}{}
	var walk func(ast.SelectionSet)
	walk = func(set ast.SelectionSet) {
		for _, selection := range set {
			switch node := selection.(type) {
			case *ast.Field:
				walk(node.SelectionSet)
			case *ast.InlineFragment:
				walk(node.SelectionSet)
			case *ast.FragmentSpread:
				names[node.Name] = struct{}{}
			}
		}
	}
	walk(selectionSet)

	out := make([]string, 0, len(names))
	for name := range names {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func (f *queryFilter) removedFields() []string {
	out := make([]string, 0, len(f.removedFieldsSet))
	for field := range f.removedFieldsSet {
		out = append(out, field)
	}
	sort.Strings(out)
	return out
}

func cloneFragmentDefinition(definition *ast.FragmentDefinition) *ast.FragmentDefinition {
	cloned := *definition
	cloned.SelectionSet = cloneSelectionSet(definition.SelectionSet)
	return &cloned
}

func cloneSelectionSet(selectionSet ast.SelectionSet) ast.SelectionSet {
	cloned := make(ast.SelectionSet, 0, len(selectionSet))
	for _, selection := range selectionSet {
		switch node := selection.(type) {
		case *ast.Field:
			field := *node
			field.SelectionSet = cloneSelectionSet(node.SelectionSet)
			cloned = append(cloned, &field)
		case *ast.InlineFragment:
			inline := *node
			inline.SelectionSet = cloneSelectionSet(node.SelectionSet)
			cloned = append(cloned, &inline)
		case *ast.FragmentSpread:
			spread := *node
			cloned = append(cloned, &spread)
		default:
			cloned = append(cloned, node)
		}
	}
	return cloned
}
