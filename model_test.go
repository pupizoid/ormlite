package ormlite

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

type model struct{}

func (m *model) Table() string { return "" }

func TestGetModelValue(t *testing.T) {
	_, err := getModelValue(&model{})
	assert.NoError(t, err)
	_, err = getModelValue(&[]*model{})
	assert.NoError(t, err)
	_, err = getModelValue(&[]struct{}{})
	assert.Error(t, err)
}
