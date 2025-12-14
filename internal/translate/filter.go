package translate

import (
	"fmt"
	"strings"

	"github.com/rkm/asf-stac-proxy/internal/asf"
)

// TranslateCQL2Filter translates a CQL2-JSON filter expression to ASF search parameters.
// This implements a subset of CQL2 focused on property comparisons that map to ASF parameters.
//
// Supported operators:
//   - "=" : equality comparison
//   - "in" : value in list
//   - "and" : logical AND
//   - "or" : logical OR
//
// Supported STAC properties:
//   - sar:instrument_mode  -> beamMode
//   - sar:polarizations    -> polarization
//   - sat:orbit_state      -> flightDirection
//   - sat:relative_orbit   -> relativeOrbit
//   - sat:absolute_orbit   -> absoluteOrbit
//   - processing:level     -> processingLevel
//   - platform             -> platform
func TranslateCQL2Filter(filter any, params *asf.SearchParams) error {
	if filter == nil {
		return nil
	}

	// Type assert to map[string]any (CQL2-JSON format)
	filterMap, ok := filter.(map[string]any)
	if !ok {
		return fmt.Errorf("%w: filter must be a JSON object", ErrUnsupportedFilter)
	}

	// Process the filter expression recursively
	return processFilterExpression(filterMap, params)
}

// processFilterExpression processes a single filter expression node
func processFilterExpression(expr map[string]any, params *asf.SearchParams) error {
	// Get the operator
	opVal, ok := expr["op"]
	if !ok {
		return fmt.Errorf("%w: missing 'op' field", ErrUnsupportedFilter)
	}

	op, ok := opVal.(string)
	if !ok {
		return fmt.Errorf("%w: 'op' must be a string", ErrUnsupportedFilter)
	}

	// Get the arguments
	argsVal, ok := expr["args"]
	if !ok {
		return fmt.Errorf("%w: missing 'args' field", ErrUnsupportedFilter)
	}

	args, ok := argsVal.([]any)
	if !ok {
		return fmt.Errorf("%w: 'args' must be an array", ErrUnsupportedFilter)
	}

	// Process based on operator
	switch strings.ToLower(op) {
	case "=", "eq":
		return processEqualityFilter(args, params)
	case "in":
		return processInFilter(args, params)
	case "and":
		return processAndFilter(args, params)
	case "or":
		return processOrFilter(args, params)
	default:
		return fmt.Errorf("%w: operator '%s' not supported", ErrUnsupportedFilter, op)
	}
}

// processEqualityFilter processes an equality comparison (=)
// Expected format: [{"property": "name"}, value]
func processEqualityFilter(args []any, params *asf.SearchParams) error {
	if len(args) != 2 {
		return fmt.Errorf("%w: '=' operator requires exactly 2 arguments", ErrUnsupportedFilter)
	}

	// Extract property name
	propName, err := extractPropertyName(args[0])
	if err != nil {
		return err
	}

	// Extract value
	value := args[1]

	// Apply the filter
	return applyPropertyFilter(propName, value, params)
}

// processInFilter processes an 'in' operator
// Expected format: [{"property": "name"}, [value1, value2, ...]]
func processInFilter(args []any, params *asf.SearchParams) error {
	if len(args) != 2 {
		return fmt.Errorf("%w: 'in' operator requires exactly 2 arguments", ErrUnsupportedFilter)
	}

	// Extract property name
	propName, err := extractPropertyName(args[0])
	if err != nil {
		return err
	}

	// Extract value list
	valueList, ok := args[1].([]any)
	if !ok {
		return fmt.Errorf("%w: second argument of 'in' must be an array", ErrUnsupportedFilter)
	}

	// Apply filter for each value in the list
	for _, value := range valueList {
		if err := applyPropertyFilter(propName, value, params); err != nil {
			return err
		}
	}

	return nil
}

// processAndFilter processes a logical AND operator
// Expected format: [expr1, expr2, ...]
func processAndFilter(args []any, params *asf.SearchParams) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: 'and' operator requires at least one argument", ErrUnsupportedFilter)
	}

	// Process each argument recursively
	for _, arg := range args {
		argMap, ok := arg.(map[string]any)
		if !ok {
			return fmt.Errorf("%w: 'and' arguments must be filter expressions", ErrUnsupportedFilter)
		}
		if err := processFilterExpression(argMap, params); err != nil {
			return err
		}
	}

	return nil
}

// processOrFilter processes a logical OR operator
// Expected format: [expr1, expr2, ...]
// Note: OR is more complex for ASF translation since most ASF parameters
// accept multiple values which are OR'd together. We handle this by
// processing each branch and collecting values for the same property.
func processOrFilter(args []any, params *asf.SearchParams) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: 'or' operator requires at least one argument", ErrUnsupportedFilter)
	}

	// For simple OR of equality on same property, process each branch
	// This naturally accumulates values in the array fields
	for _, arg := range args {
		argMap, ok := arg.(map[string]any)
		if !ok {
			return fmt.Errorf("%w: 'or' arguments must be filter expressions", ErrUnsupportedFilter)
		}
		if err := processFilterExpression(argMap, params); err != nil {
			return err
		}
	}

	return nil
}

// extractPropertyName extracts the property name from a property reference
// Expected format: {"property": "name"}
func extractPropertyName(arg any) (string, error) {
	propMap, ok := arg.(map[string]any)
	if !ok {
		return "", fmt.Errorf("%w: property reference must be an object", ErrUnsupportedFilter)
	}

	propVal, ok := propMap["property"]
	if !ok {
		return "", fmt.Errorf("%w: missing 'property' field in property reference", ErrUnsupportedFilter)
	}

	propName, ok := propVal.(string)
	if !ok {
		return "", fmt.Errorf("%w: 'property' must be a string", ErrUnsupportedFilter)
	}

	return propName, nil
}

// applyPropertyFilter applies a property filter to ASF search parameters
func applyPropertyFilter(propName string, value any, params *asf.SearchParams) error {
	// Map STAC property names to ASF parameters
	switch propName {
	case "sar:instrument_mode":
		return applyBeamModeFilter(value, params)
	case "sar:polarizations":
		return applyPolarizationFilter(value, params)
	case "sar:product_type":
		// sar:product_type maps to ASF's processingLevel (e.g., SLC, GRD, RAW)
		return applyProcessingLevelFilter(value, params)
	case "sat:orbit_state":
		return applyOrbitStateFilter(value, params)
	case "sat:relative_orbit":
		return applyRelativeOrbitFilter(value, params)
	case "sat:absolute_orbit":
		return applyAbsoluteOrbitFilter(value, params)
	case "processing:level":
		return applyProcessingLevelFilter(value, params)
	case "platform":
		return applyPlatformFilter(value, params)
	default:
		return fmt.Errorf("%w: property '%s' not supported for filtering", ErrUnsupportedFilter, propName)
	}
}

// applyBeamModeFilter applies a beam mode filter (sar:instrument_mode -> beamMode)
func applyBeamModeFilter(value any, params *asf.SearchParams) error {
	strValue, ok := value.(string)
	if !ok {
		return fmt.Errorf("%w: sar:instrument_mode value must be a string", ErrUnsupportedFilter)
	}

	// Add to beam mode list (ASF accepts multiple values as OR)
	params.BeamMode = append(params.BeamMode, strValue)
	return nil
}

// applyPolarizationFilter applies a polarization filter (sar:polarizations -> polarization)
func applyPolarizationFilter(value any, params *asf.SearchParams) error {
	strValue, ok := value.(string)
	if !ok {
		return fmt.Errorf("%w: sar:polarizations value must be a string", ErrUnsupportedFilter)
	}

	// Add to polarization list (ASF accepts multiple values as OR)
	params.Polarization = append(params.Polarization, strValue)
	return nil
}

// applyOrbitStateFilter applies an orbit state filter (sat:orbit_state -> flightDirection)
// Transforms lowercase values to uppercase for ASF
func applyOrbitStateFilter(value any, params *asf.SearchParams) error {
	strValue, ok := value.(string)
	if !ok {
		return fmt.Errorf("%w: sat:orbit_state value must be a string", ErrUnsupportedFilter)
	}

	// Convert to uppercase for ASF (ASCENDING, DESCENDING)
	asfValue := strings.ToUpper(strValue)

	// Validate the value
	if asfValue != "ASCENDING" && asfValue != "DESCENDING" {
		return fmt.Errorf("%w: sat:orbit_state must be 'ascending' or 'descending'", ErrUnsupportedFilter)
	}

	// ASF only accepts a single flight direction value (not a list)
	// If already set and different, this is a conflict
	if params.FlightDirection != "" && params.FlightDirection != asfValue {
		return fmt.Errorf("%w: conflicting flight direction values", ErrUnsupportedFilter)
	}

	params.FlightDirection = asfValue
	return nil
}

// applyRelativeOrbitFilter applies a relative orbit filter (sat:relative_orbit -> relativeOrbit)
func applyRelativeOrbitFilter(value any, params *asf.SearchParams) error {
	// Handle both float64 (JSON numbers) and int
	var intValue int
	switch v := value.(type) {
	case float64:
		intValue = int(v)
	case int:
		intValue = v
	default:
		return fmt.Errorf("%w: sat:relative_orbit value must be a number", ErrUnsupportedFilter)
	}

	// Add to relative orbit list (ASF accepts multiple values as OR)
	params.RelativeOrbit = append(params.RelativeOrbit, intValue)
	return nil
}

// applyAbsoluteOrbitFilter applies an absolute orbit filter (sat:absolute_orbit -> absoluteOrbit)
func applyAbsoluteOrbitFilter(value any, params *asf.SearchParams) error {
	// Handle both float64 (JSON numbers) and int
	var intValue int
	switch v := value.(type) {
	case float64:
		intValue = int(v)
	case int:
		intValue = v
	default:
		return fmt.Errorf("%w: sat:absolute_orbit value must be a number", ErrUnsupportedFilter)
	}

	// Add to absolute orbit list (ASF accepts multiple values as OR)
	params.AbsoluteOrbit = append(params.AbsoluteOrbit, intValue)
	return nil
}

// applyProcessingLevelFilter applies a processing level filter (processing:level -> processingLevel)
func applyProcessingLevelFilter(value any, params *asf.SearchParams) error {
	strValue, ok := value.(string)
	if !ok {
		return fmt.Errorf("%w: processing:level value must be a string", ErrUnsupportedFilter)
	}

	// Add to processing level list (ASF accepts multiple values as OR)
	params.ProcessingLevel = append(params.ProcessingLevel, strValue)
	return nil
}

// applyPlatformFilter applies a platform filter (platform -> platform)
func applyPlatformFilter(value any, params *asf.SearchParams) error {
	strValue, ok := value.(string)
	if !ok {
		return fmt.Errorf("%w: platform value must be a string", ErrUnsupportedFilter)
	}

	// Add to platform list (ASF accepts multiple values as OR)
	params.Platform = append(params.Platform, strValue)
	return nil
}
