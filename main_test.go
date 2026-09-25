package main

import (
	"reflect"
	"testing"
)

func TestArrayBySerial(t *testing.T) {
	in := `{"system_id":1,"arrays":[
		{"label":"Array 1","modules":[{"inverter":{"serial_num":"a1"}},{"inverter":{"serial_num":"a2"}}]},
		{"label":"Array 2","modules":[{"inverter":{"serial_num":"b1"}}]}]}`
	got, err := arrayBySerial([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"a1": "Array 1", "a2": "Array 1", "b1": "Array 2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
