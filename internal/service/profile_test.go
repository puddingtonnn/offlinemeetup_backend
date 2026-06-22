package service

import (
	"context"
	"errors"
	"testing"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// --- 1. Создаем моки для интерфейсов ---

// MockProfileRepo имитирует интерфейс ProfileRepository
type MockProfileRepo struct {
	mock.Mock
}

func (m *MockProfileRepo) GetByUserID(ctx context.Context, userID int64) (*domain.Profile, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Profile), args.Error(1)
}

func (m *MockProfileRepo) UpdateProfile(ctx context.Context, profile *domain.Profile) (*domain.Profile, error) {
	args := m.Called(ctx, profile)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Profile), args.Error(1)
}

// MockUserTagUpdater имитирует интерфейс UserTagUpdater
type MockUserTagUpdater struct {
	mock.Mock
}

func (m *MockUserTagUpdater) UpdateTags(ctx context.Context, userID int64, tagIDs []int64) error {
	args := m.Called(ctx, userID, tagIDs)
	return args.Error(0)
}

func (m *MockUserTagUpdater) GetTagsByUserID(ctx context.Context, userID int64) ([]domain.Tag, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.Tag), args.Error(1)
}

// --- 2. Пишем табличные тесты ---

func TestProfileService_GetProfile(t *testing.T) {
	// Подготавливаем тестовые данные (Arrange)
	testUserID := int64(1)

	// Таблица тестовых сценариев
	tests := []struct {
		name          string                                                   // Название теста
		mockSetup     func(repo *MockProfileRepo, tagRepo *MockUserTagUpdater) // Настройка моков
		expectedError error                                                    // Ожидаемая ошибка (если есть)
		checkResult   func(t *testing.T, profile *dto.ProfileResponse)         // Функция для проверки результата (если нет ошибки)
	}{
		{
			name: "Успешное получение профиля",
			mockSetup: func(repo *MockProfileRepo, tagRepo *MockUserTagUpdater) {
				// Указываем: когда вызовут GetByUserID с нашими параметрами, верни профиль и nil ошибку
				repo.On("GetByUserID", mock.Anything, testUserID).
					Return(&domain.Profile{ID: 100, UserID: testUserID, Nickname: "john_doe"}, nil)

				// Указываем: когда вызовут GetTagsByUserID, верни массив тегов
				tagRepo.On("GetTagsByUserID", mock.Anything, testUserID).
					Return([]domain.Tag{{ID: 1, Name: "Golang"}}, nil)
			},
			expectedError: nil,
			checkResult: func(t *testing.T, profile *dto.ProfileResponse) {
				assert.NotNil(t, profile)
				assert.Equal(t, "john_doe", profile.Nickname)
				assert.Len(t, profile.Tags, 1)
				assert.Equal(t, "Golang", profile.Tags[0].Name)
			},
		},
		{
			name: "Профиль не найден",
			mockSetup: func(repo *MockProfileRepo, tagRepo *MockUserTagUpdater) {
				// Имитируем, что профиля в базе нет
				repo.On("GetByUserID", mock.Anything, testUserID).
					Return(nil, nil)
			},
			expectedError: ErrNotFound,
		},
		{
			name: "Ошибка базы данных при получении профиля",
			mockSetup: func(repo *MockProfileRepo, tagRepo *MockUserTagUpdater) {
				// Имитируем разрыв соединения с БД
				repo.On("GetByUserID", mock.Anything, testUserID).
					Return(nil, errors.New("db connection failed"))
			},
			expectedError: errors.New("db connection failed"),
		},
	}

	// Запускаем каждый тест из таблицы
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Инициализируем моки для каждого теста
			mockProfileRepo := new(MockProfileRepo)
			mockTagRepo := new(MockUserTagUpdater)

			// Применяем настройки моков (что они должны вернуть в этом конкретном сценарии)
			tt.mockSetup(mockProfileRepo, mockTagRepo)

			// Создаем сервис, передавая ему наши моки вместо реальной базы!
			service := NewProfileService(mockProfileRepo, mockTagRepo, "https://s3.local")

			// Вызываем тестируемый метод (Act)
			result, err := service.GetProfile(context.Background(), testUserID)

			// Проверяем результаты (Assert)
			if tt.expectedError != nil {
				// Если ждали ошибку, проверяем что она вернулась (используем assert.ErrorIs или Contains)
				assert.Error(t, err)
				if tt.expectedError == ErrNotFound {
					assert.ErrorIs(t, err, ErrNotFound)
				}
				assert.Nil(t, result)
			} else {
				// Если не ждали ошибку, проверяем что ее нет и данные корректны
				assert.NoError(t, err)
				tt.checkResult(t, result)
			}

			// Проверяем, что все методы у моков, которые мы ожидали, действительно были вызваны
			mockProfileRepo.AssertExpectations(t)
			mockTagRepo.AssertExpectations(t)
		})
	}
}
