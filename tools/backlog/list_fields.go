package main

import (
	"errors"
	"flag"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

func showFields(fs *flag.FlagSet, format outputFormat, value string, sample any) []jsonField {
	if !flagSeen(fs, "fields") {
		return nil
	}
	if format != "json" {
		fatal(errors.New("--fields requires --format json"))
	}
	fields, err := parseJSONFields(value, sample)
	if err != nil {
		fatal(err)
	}
	return fields
}

type jsonField struct {
	name  string
	index int
}

func parseCardListFields(value string) ([]jsonField, error) {
	return parseJSONFields(value, Card{}, "sections", "history", "target_assessments")
}

func parseJSONFields(value string, sample any, excluded ...string) ([]jsonField, error) {
	available := make(map[string]int)
	t := reflect.TypeOf(sample)
	for i := 0; i < t.NumField(); i++ {
		name := strings.Split(t.Field(i).Tag.Get("json"), ",")[0]
		skip := name == "" || name == "-"
		for _, field := range excluded {
			skip = skip || name == field
		}
		if skip {
			continue
		}
		available[name] = i
	}
	selected := make([]jsonField, 0)
	seen := make(map[string]bool)
	for _, part := range strings.Split(value, ",") {
		name := strings.TrimSpace(part)
		index, ok := available[name]
		if !ok {
			names := make([]string, 0, len(available))
			for field := range available {
				names = append(names, field)
			}
			sort.Strings(names)
			return nil, fmt.Errorf("invalid --fields entry %q; available fields: %s", name, strings.Join(names, ","))
		}
		if !seen[name] {
			selected = append(selected, jsonField{name, index})
			seen[name] = true
		}
	}
	return selected, nil
}

func projectCardList(cards []Card, fields []jsonField) []map[string]any {
	rows := make([]map[string]any, 0, len(cards))
	for _, card := range cards {
		rows = append(rows, projectJSONFields(card, fields))
	}
	return rows
}

func projectJSONFields(entity any, fields []jsonField) map[string]any {
	value := reflect.ValueOf(entity)
	row := make(map[string]any, len(fields))
	for _, field := range fields {
		row[field.name] = value.Field(field.index).Interface()
	}
	return row
}
