package corefunctions

import (
	"context"

	"github.com/two-hundred/celerity/libs/blueprint/function"
	"github.com/two-hundred/celerity/libs/blueprint/provider"
	. "gopkg.in/check.v1"
)

type TrimPrefix_G_FunctionTestSuite struct {
	callStack   function.Stack
	callContext *functionCallContextMock
}

var _ = Suite(&TrimPrefix_G_FunctionTestSuite{})

func (s *TrimPrefix_G_FunctionTestSuite) SetUpTest(c *C) {
	s.callStack = function.NewStack()
	s.callContext = &functionCallContextMock{
		params: &blueprintParamsMock{},
		registry: &functionRegistryMock{
			functions: map[string]provider.Function{},
			callStack: s.callStack,
		},
		callStack: s.callStack,
	}
}

func (s *TrimPrefix_G_FunctionTestSuite) Test_returns_function_runtime_info_with_partial_args(c *C) {
	trimPrefix_G_Func := NewTrimPrefix_G_Function()
	s.callStack.Push(&function.Call{
		FunctionName: "trimprefix_g",
	})
	output, err := trimPrefix_G_Func.Call(context.TODO(), &provider.FunctionCallInput{
		Arguments: &functionCallArgsMock{
			args: []any{
				"https://",
			},
			callCtx: s.callContext,
		},
		CallContext: s.callContext,
	})

	c.Assert(err, IsNil)
	c.Assert(output.FunctionInfo, DeepEquals, provider.FunctionRuntimeInfo{
		FunctionName: "trimprefix",
		PartialArgs:  []any{"https://"},
		ArgsOffset:   1,
	})
}
