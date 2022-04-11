package main

import (
	"context"
	"errors"

	"golang.org/x/xerrors"

	addr "github.com/filecoin-project/go-address"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
)

/*
Rule examples:
	- Accept only from f3aaa to f0123
		{
		  "Source": {
			"Addr": ["f3aaa"],
			"Next": {
			  "Destination": {
				"Addr": ["f0123"],
				"Next": {"Accept": {}}
			  }
			}
		  }
		}
	- Accept only from f3aaa to f0123 using And
		{
		  "And": [
			{"Source": {"Addr": ["f3aaa"], "Next": {"Accept": {}}}},
			{"Destination": {"Addr": ["f0123"], "Next": {"Accept": {}}}}
		  ]
		}

(AnyAccepts ([
	(Has (Addr ([f0234]) (Accept)))
	(Sign Or[(Block (Reject)) (Message (To ([f0555]) (Accept)))])
]))


*/

type FilterParam string

const (
	ParamAction   FilterParam = "Action"
	ParamSignType FilterParam = "SignType" // requires Action=Sign

	ParamSource      FilterParam = "Source"      // requires SignType=Message
	ParamDestination FilterParam = "Destination" // requires SignType=Message
	ParamMethod      FilterParam = "Method"      // requires SignType=Message
	ParamValue       FilterParam = "Value"       // requires SignType=Message
	ParamMaxFee      FilterParam = "MaxFee"      // requires SignType=Message

	ParamMiner  FilterParam = "Miner"  // requires SignType=Message
	ParamSigner FilterParam = "Signer" // requires SignType=Message
)

type Action string

const (
	ActionNew  Action = "New"
	ActionHas  Action = "Has"
	ActionList Action = "List"
	ActionSign Action = "Sign"

	ActionExport Action = "Export"
	ActionImport Action = "Import"
	ActionDelete Action = "Delete"
)

type SignType string

const (
	SignTypeMessage      SignType = "Message"
	SignTypeBlock        SignType = "Block"
	SignTypeDealProposal SignType = "DealProposal"
)

type Rule interface{}

type Filter func(map[FilterParam]interface{}) error
type RuleParser func(context.Context, Rule) (Filter, error)

var ruleParsers map[string]RuleParser

func init() {
	ruleParsers = map[string]RuleParser{
		"AnyAccepts": AnyAccepts,

		"New":  action(ActionNew),
		"Sign": action(ActionSign),

		"Signer": checkRule(ParamAction, ActionSign, AddrRule(ParamSigner)),

		"Message":     checkRule(ParamAction, ActionSign, Message),
		"Source":      checkRule(ParamSignType, SignTypeMessage, AddrRule(ParamSource)),
		"Destination": checkRule(ParamSignType, SignTypeMessage, AddrRule(ParamDestination)),
		"Method":      checkRule(ParamSignType, SignTypeMessage, MethodRule),
		"Value":       checkRule(ParamSignType, SignTypeMessage, ValueRule(ParamValue)),
		"MaxFee":      checkRule(ParamSignType, SignTypeMessage, ValueRule(ParamMaxFee)),

		"Block": checkRule(ParamAction, ActionSign, Block),
		"Miner": checkRule(ParamSignType, SignTypeBlock, AddrRule(ParamMiner)),

		"DealProposal": checkRule(ParamAction, ActionSign, DealProposal),

		"Accept": finalRule(ErrAccept),
		"Reject": finalRule(ErrReject),
	}
}

// Core rules

var ErrAccept = errors.New("filter accept")
var ErrReject = errors.New("filter reject")

// finalRule is either accept or reject
// r: {}
func finalRule(err error) func(ctx context.Context, r Rule) (Filter, error) {
	return func(ctx context.Context, r Rule) (Filter, error) {
		rules, ok := r.(map[string]interface{})
		if !ok {
			return nil, xerrors.Errorf("expected final rule to be a map")
		}

		if len(rules) != 0 {
			return nil, xerrors.Errorf("final rule must be an empty map, had %d elements", rules)
		}

		return func(params map[FilterParam]interface{}) error {
			return err
		}, nil
	}
}

// AnyAccepts accepts when at least one subrule accepts; Stops on first Accept or Reject
// r: [Rule...]
func AnyAccepts(ctx context.Context, r Rule) (Filter, error) {
	ruleList, ok := r.([]interface{})
	if !ok {
		return nil, xerrors.Errorf("expected AnyAccept to be an array")
	}

	next := make([]Filter, len(ruleList))

	for i, subRule := range ruleList {
		sub, err := ParseRule(ctx, subRule)
		if err != nil {
			return nil, xerrors.Errorf("parsing subrule %d: %w", i, err)
		}

		next[i] = sub
	}

	return func(params map[FilterParam]interface{}) error {
		for _, filter := range next {
			if err := filter(params); err != nil {
				return err
			}
		}

		return nil
	}, nil
}

// ParseRule parses a single rule
// r: {"ruleName": ...}
func ParseRule(ctx context.Context, r Rule) (Filter, error) {
	rules, ok := r.(map[string]interface{})
	if !ok {
		return nil, xerrors.Errorf("expected rule to be a map, was %T", r)
	}

	if len(rules) != 1 {
		return nil, xerrors.Errorf("expected one rule, had %d", len(rules))
	}

	for p, r := range rules {
		subParser, found := ruleParsers[p]
		if !found {
			return nil, xerrors.Errorf("rule '%s' not found", p)
		}

		return subParser(ctx, r)
	}

	// can't be reached
	return nil, xerrors.Errorf("no rules?")
}

func checkRule(fp FilterParam, is interface{}, sub RuleParser) RuleParser {
	return func(ctx context.Context, rule Rule) (Filter, error) {
		if ctx.Value(fp) != is {
			return nil, xerrors.Errorf("bad rule context") // todo nicer error
		}

		return sub(ctx, rule)
	}
}

// With action

// action matches specified action
// r: {"ruleName": ...}
func action(action Action) func(ctx context.Context, r Rule) (Filter, error) {
	return func(ctx context.Context, r Rule) (Filter, error) {
		ctx = context.WithValue(ctx, ParamAction, action)

		sub, err := ParseRule(ctx, r)
		if err != nil {
			return nil, xerrors.Errorf("parsing next rule: %w", err)
		}

		return func(params map[FilterParam]interface{}) error {
			if params[ParamAction] != action {
				return nil
			}

			return sub(params)
		}, nil
	}
}

// Sign type filters

func Message(ctx context.Context, r Rule) (Filter, error) {
	ctx = context.WithValue(ctx, ParamSignType, SignTypeMessage)

	sub, err := ParseRule(ctx, r)
	if err != nil {
		return nil, xerrors.Errorf("parsing next rule: %w", err)
	}

	return func(params map[FilterParam]interface{}) error {
		if params[ParamSignType] != api.MTChainMsg {
			return nil
		}

		return sub(params)
	}, nil
}

func Block(ctx context.Context, r Rule) (Filter, error) {
	ctx = context.WithValue(ctx, ParamSignType, SignTypeBlock)

	sub, err := ParseRule(ctx, r)
	if err != nil {
		return nil, xerrors.Errorf("parsing next rule %d: %w", err)
	}

	return func(params map[FilterParam]interface{}) error {
		if params[ParamSignType] != api.MTBlock {
			return nil
		}

		return sub(params)
	}, nil
}

func DealProposal(ctx context.Context, r Rule) (Filter, error) {
	ctx = context.WithValue(ctx, ParamSignType, SignTypeDealProposal)

	sub, err := ParseRule(ctx, r)
	if err != nil {
		return nil, xerrors.Errorf("parsing next rule: %w", err)
	}

	return func(params map[FilterParam]interface{}) error {
		if params[ParamSignType] != api.MTDealProposal {
			return nil
		}

		return sub(params)
	}, nil
}

// Message params

// AddrRule checks for addresses in a set
// r: {"Addr": [addrs..], "Next": Rule}
func AddrRule(param FilterParam) func(ctx context.Context, r Rule) (Filter, error) {
	return func(ctx context.Context, r Rule) (Filter, error) {

		fields, ok := r.(map[string]interface{})
		if !ok {
			return nil, xerrors.Errorf("expected addr rule to be a map")
		}

		if len(fields) != 2 {
			return nil, xerrors.Errorf("expected addr rule to have 2 fields, had %d", len(fields))
		}

		sr, ok := fields["Next"]
		if !ok {
			return nil, xerrors.Errorf("addr rule didn't have 'Next' field")
		}

		sub, err := ParseRule(ctx, sr)
		if err != nil {
			return nil, xerrors.Errorf("parsing address subrule: %w", err)
		}

		listField, ok := fields["Addr"]
		if !ok {
			return nil, xerrors.Errorf("addr rule didn't have 'Addr' field")
		}

		addrList, ok := listField.([]interface{})
		if !ok {
			// todo maybe allow a string
			return nil, xerrors.Errorf("'Addr' field must be a list")
		}

		addrs := make(map[addr.Address]struct{}, len(addrList))
		for i, ai := range addrList {
			as, ok := ai.(string)
			if !ok {
				return nil, xerrors.Errorf("address %d in address list wasn't a string", i)
			}
			a, err := addr.NewFromString(as)
			if err != nil {
				return nil, xerrors.Errorf("failed to parse address %d in list: %w", i, err)
			}
			addrs[a] = struct{}{}
		}

		return func(params map[FilterParam]interface{}) error {
			ai, ok := params[param]
			if !ok {
				return xerrors.Errorf("address param not set")
			}
			a, ok := ai.(addr.Address)
			if !ok {
				return xerrors.Errorf("address param with wrong type")
			}
			_, has := addrs[a]
			if !has {
				return nil
			}

			return sub(params)
		}, nil
	}
}

// MethodRule checks for methods in a set
// r: {"Method": [number..], "Next": Rule}
func MethodRule(ctx context.Context, r Rule) (Filter, error) {
	fields, ok := r.(map[string]interface{})
	if !ok {
		return nil, xerrors.Errorf("expected method rule to be a map")
	}

	if len(fields) != 2 {
		return nil, xerrors.Errorf("expected method rule to have 2 fields, had %d", len(fields))
	}

	sr, ok := fields["Next"]
	if !ok {
		return nil, xerrors.Errorf("method rule didn't have 'Next' field")
	}

	sub, err := ParseRule(ctx, sr)
	if err != nil {
		return nil, xerrors.Errorf("parsing method subrule: %w", err)
	}

	listField, ok := fields["Method"]
	if !ok {
		return nil, xerrors.Errorf("method rule didn't have 'Method' field")
	}

	methodList, ok := listField.([]interface{})
	if !ok {
		// todo maybe allow a number
		return nil, xerrors.Errorf("'Method' field must be a list")
	}

	methods := make(map[abi.MethodNum]struct{}, len(methodList))
	for i, ai := range methodList {
		as, ok := ai.(float64)
		if !ok {
			return nil, xerrors.Errorf("address %d in address list wasn't a number", i)
		}
		methods[abi.MethodNum(as)] = struct{}{}
	}

	return func(params map[FilterParam]interface{}) error {
		ai, ok := params[ParamMethod]
		if !ok {
			return xerrors.Errorf("method param not set")
		}
		a, ok := ai.(abi.MethodNum)
		if !ok {
			return xerrors.Errorf("method param with wrong type")
		}
		_, has := methods[a]
		if !has {
			return nil
		}

		return sub(params)
	}, nil
}

// ValueRule checks message value
// r: {("LessThan"|"MoreThan"): "[fil]", "Next": Rule}
func ValueRule(param FilterParam) func(ctx context.Context, r Rule) (Filter, error) {
	return func(ctx context.Context, r Rule) (Filter, error) {
		fields, ok := r.(map[string]interface{})
		if !ok {
			return nil, xerrors.Errorf("expected value rule to be a map")
		}

		if len(fields) != 2 {
			return nil, xerrors.Errorf("expected value rule to have 2 fields, had %d", len(fields))
		}

		sr, ok := fields["Next"]
		if !ok {
			return nil, xerrors.Errorf("value rule didn't have 'Next' field")
		}

		sub, err := ParseRule(ctx, sr)
		if err != nil {
			return nil, xerrors.Errorf("parsing value subrule: %w", err)
		}

		less := true
		valField, ok := fields["LessThan"]
		if !ok {
			valField, ok = fields["MoreThan"]
			less = false

			if !ok {
				return nil, xerrors.Errorf("value rule must have one of 'LessThan' or 'MoreThan' fields")
			}
		}

		vstr, ok := valField.(string)
		if !ok {
			return nil, xerrors.Errorf("value field wasn't a string")
		}

		val, err := types.ParseFIL(vstr)
		if err != nil {
			return nil, xerrors.Errorf("parsing fil value: %w", err)
		}

		return func(params map[FilterParam]interface{}) error {
			ai, ok := params[param]
			if !ok {
				return xerrors.Errorf("method param not set")
			}
			a, ok := ai.(abi.TokenAmount)
			if !ok {
				return xerrors.Errorf("value param with wrong type")
			}
			if (less && a.GreaterThan(abi.TokenAmount(val))) ||
				(!less && a.LessThan(abi.TokenAmount(val))) {
				return nil
			}
			return sub(params)
		}, nil
	}
}
