package domain_test

import (
	"encoding/json"
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestBMSTableHeader_UnmarshalJSON_StringLevelOrder(t *testing.T) {
	src := []byte(`{
		"name":"Satellite","symbol":"sl","data_url":"satellite_data.json",
		"level_order":["0","1","2","3"]
	}`)
	var h domain.BMSTableHeader
	require.NoError(t, json.Unmarshal(src, &h))
	require.Equal(t, "Satellite", h.Name)
	require.Equal(t, "sl", h.Symbol)
	require.Equal(t, "satellite_data.json", h.DataURL)
	require.Equal(t, []string{"0", "1", "2", "3"}, h.LevelOrder)
}

func TestBMSTableHeader_UnmarshalJSON_NumberLevelOrder(t *testing.T) {
	src := []byte(`{
		"name":"X","symbol":"x","data_url":"data.json",
		"level_order":[0,1,2.5,3]
	}`)
	var h domain.BMSTableHeader
	require.NoError(t, json.Unmarshal(src, &h))
	require.Equal(t, []string{"0", "1", "2.5", "3"}, h.LevelOrder)
}

func TestBMSTableHeader_UnmarshalJSON_MissingLevelOrderIsNil(t *testing.T) {
	src := []byte(`{"name":"Y","symbol":"y","data_url":"d.json"}`)
	var h domain.BMSTableHeader
	require.NoError(t, json.Unmarshal(src, &h))
	require.Nil(t, h.LevelOrder)
}

func TestBMSTableHeader_UnmarshalJSON_RejectsMixedArray(t *testing.T) {
	src := []byte(`{"name":"Z","symbol":"z","data_url":"d.json","level_order":["0",1]}`)
	var h domain.BMSTableHeader
	require.Error(t, json.Unmarshal(src, &h))
}
