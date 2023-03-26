package src

import (
	"errors"
	"github.com/sashabaranov/go-openai"
)

const LogicErrorContextLengthExceeded = "context_length_exceeded"

type LogicError struct {
	Code    string
	Message string
}

func (e LogicError) Error() string {
	return e.Code
}

func getOpenAIErrorCode(err error) string {
	if unwrapped := errors.Unwrap(err); unwrapped != nil {
		if apiErr, ok := unwrapped.(*openai.APIError); ok {
			if apiErr.Code != nil {
				return *apiErr.Code
			}
		}
	}

	return ""
}

func GetLogicErrorCode(err error) string {
	if logicErr, ok := err.(LogicError); ok {
		return logicErr.Code
	}

	return ""
}
