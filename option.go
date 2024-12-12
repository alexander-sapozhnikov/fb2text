package fb2text

type option struct {
	parseBody       bool
	skipSystemLines bool
}

type FOption func(option) option

var Option option

func ParseBody() FOption {
	return func(o option) option {
		o.parseBody = true
		return o
	}
}

func SkipSystemLines() FOption {
	return func(o option) option {
		o.skipSystemLines = true
		return o
	}
}
