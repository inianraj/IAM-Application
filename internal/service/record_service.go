package service

import (
	"context"

	"crud/internal/domain"
)

type TaskStore interface {
	Create(ctx context.Context, t domain.Task) (domain.Task, error)
	GetByID(ctx context.Context, id int64) (domain.Task, error)
	List(ctx context.Context) ([]domain.Task, error)
	Update(ctx context.Context, t domain.Task) (domain.Task, error)
	Delete(ctx context.Context, id int64) error
}

type TaskService struct {
	store TaskStore
}

func NewTaskService(store TaskStore) *TaskService {
	return &TaskService{store: store}
}

func checkID(id int64) error {
	if id <= 0 {
		return domain.ErrInvalidTaskID
	}
	return nil
}

func (s *TaskService) Create(ctx context.Context, t domain.Task) (domain.Task, error) {
	if err := t.Validate(); err != nil {
		return domain.Task{}, err
	}
	return s.store.Create(ctx, t)
}

func (s *TaskService) Get(ctx context.Context, id int64) (domain.Task, error) {
	if err := checkID(id); err != nil {
		return domain.Task{}, err
	}
	return s.store.GetByID(ctx, id)
}

func (s *TaskService) List(ctx context.Context) ([]domain.Task, error) {
	return s.store.List(ctx)
}

func (s *TaskService) Update(ctx context.Context, t domain.Task) (domain.Task, error) {
	if err := checkID(t.ID); err != nil {
		return domain.Task{}, err
	}
	if err := t.Validate(); err != nil {
		return domain.Task{}, err
	}
	return s.store.Update(ctx, t)
}

func (s *TaskService) Delete(ctx context.Context, id int64) error {
	if err := checkID(id); err != nil {
		return err
	}
	return s.store.Delete(ctx, id)
}
