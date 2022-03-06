package handler

// execute.go handles the execution of a GraphQL request

import (
	"context"
	"fmt"
	"github.com/dolmen-go/jsonmap"
	"github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/gqlerror"
	"github.com/vektah/gqlparser/parser"
	"github.com/vektah/gqlparser/validator"
	"reflect"
)

type (
	// gqlRequest decodes and handles each GraphQL request
	gqlRequest struct {
		h *Handler

		// These are decoded from the http request body (JSON)
		Query         string
		OperationName string
		Variables     map[string]interface{}
	}

	// gqlResult contains the result (or errors) of the request to be encoded in JSON
	gqlResult struct {
		//Data   map[string]interface{} `json:"data,omitempty"`
		Data   jsonmap.Ordered `json:"data,omitempty"`
		Errors gqlerror.List   `json:"errors,omitempty"`
	}
)

// Execute parses and runs the request and returns the result
func (g *gqlRequest) Execute(ctx context.Context) (r gqlResult) {
	// First analyse and validate the query string
	query, pgqlError := parser.ParseQuery(&ast.Source{
		Name:  "query",
		Input: g.Query,
	})
	if pgqlError != nil {
		r.Errors = append(r.Errors, pgqlError)
		return
	}

	r.Errors = validator.Validate(g.h.schema, query)
	if r.Errors != nil {
		return
	}

	// Now process the operation(s)
	r.Data.Data = make(map[string]interface{})
	for _, operation := range query.Operations {
		op := gqlOperation{enums: g.h.enums}

		// Get variables associated with this operation if any
		if len(operation.VariableDefinitions) > 0 {
			if op.variables, pgqlError = validator.VariableValues(g.h.schema, operation, g.Variables); pgqlError != nil {
				r.Errors = append(r.Errors, pgqlError)
				continue // skip this op if we can't get the vars
			}
		}

		var v, vIntro reflect.Value // value of the root query or mutation
		var introOp *gqlOperation
		switch operation.Operation {
		case ast.Query:
			v = reflect.ValueOf(g.h.qData)
			if AllowIntrospection {
				introOp = &gqlOperation{enums: IntrospectionEnums}
				vIntro = reflect.ValueOf(NewIntrospectionData(g.h.schema))
			}
		case ast.Mutation:
			op.isMutation = true
			v = reflect.ValueOf(g.h.mData)
		case ast.Subscription:
			r.Errors = append(r.Errors, &gqlerror.Error{Message: "TODO: subscriptions not yet implemented"})
			return
		default:
			panic("unexpected")
		}
		result, err := op.GetSelections(ctx, operation.SelectionSet, v, vIntro, introOp)

		// TODO: don't stop on 1st error but record all errors to save the client debug time
		if err != nil {
			r.Errors = append(r.Errors, &gqlerror.Error{
				Message:    err.Error(),
				Extensions: map[string]interface{}{"operation": operation.Name},
			})
			return
		}
		for _, k := range result.Order {
			if _, ok := r.Data.Data[k]; ok {
				r.Errors = append(r.Errors, &gqlerror.Error{
					Message:    fmt.Sprintf("resolver %q in %s has duplicate name", k, operation.Name),
					Extensions: map[string]interface{}{"operation": operation.Name},
				})
				return
			}
			r.Data.Data[k] = result.Data[k]
		}
		r.Data.Order = append(r.Data.Order, result.Order...)
		if len(r.Data.Order) != len(r.Data.Data) {
			panic("map and slice in the jsonmap.Ordered should be the same size")
		}
	}
	return
}
