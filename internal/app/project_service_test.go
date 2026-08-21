package app

import (
	"context"
	"errors"
	"testing"

	"github.com/hrtsegv/flagship/internal/domain"
	"github.com/hrtsegv/flagship/internal/memory"
)

func TestProjectService_Create(t *testing.T) {
	ctx := context.Background()
	svc := NewProjectService(memory.NewProjectRepository())

	p, err := svc.Create(ctx, CreateProjectInput{Key: "ecommerce", Name: "Ecommerce", Description: "Store"})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if p.ID == "" {
		t.Error("Create() did not assign an ID")
	}
	if p.Key != "ecommerce" || p.Name != "Ecommerce" || p.Description != "Store" {
		t.Errorf("Create() = %+v, unexpected fields", p)
	}
	if p.CreatedAt.IsZero() || p.UpdatedAt.IsZero() {
		t.Error("Create() did not set timestamps")
	}
}

func TestProjectService_Create_DuplicateKey(t *testing.T) {
	ctx := context.Background()
	svc := NewProjectService(memory.NewProjectRepository())

	if _, err := svc.Create(ctx, CreateProjectInput{Key: "ecommerce", Name: "Ecommerce"}); err != nil {
		t.Fatalf("first Create() unexpected error: %v", err)
	}
	_, err := svc.Create(ctx, CreateProjectInput{Key: "ecommerce", Name: "Ecommerce Again"})
	if !errors.Is(err, domain.ErrDuplicateKey) {
		t.Fatalf("second Create() error = %v, want ErrDuplicateKey", err)
	}
}

func TestProjectService_Create_Validation(t *testing.T) {
	ctx := context.Background()
	svc := NewProjectService(memory.NewProjectRepository())

	tests := []struct {
		name  string
		input CreateProjectInput
	}{
		{"missing key", CreateProjectInput{Name: "Ecommerce"}},
		{"missing name", CreateProjectInput{Key: "ecommerce"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Create(ctx, tt.input)
			if !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("Create() error = %v, want ErrValidation", err)
			}
		})
	}
}

func TestProjectService_List(t *testing.T) {
	ctx := context.Background()
	svc := NewProjectService(memory.NewProjectRepository())

	if _, err := svc.Create(ctx, CreateProjectInput{Key: "a", Name: "A"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, CreateProjectInput{Key: "b", Name: "B"}); err != nil {
		t.Fatal(err)
	}

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("List() returned %d projects, want 2", len(list))
	}
}
