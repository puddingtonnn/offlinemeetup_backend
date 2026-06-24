package cache

import "time"

// Metrics наблюдает за поведением кэша. Объявлен у потребителя (package cache),
// чтобы реализация (Prometheus) жила в отдельном пакете и не тянула сюда
// зависимость. name — имя кэша (chats/tags/profile/meetup), op — "get"/"set".
type Metrics interface {
	Hit(name string)
	Miss(name string)
	Error(name string)
	ObserveLatency(name, op string, d time.Duration)
}

// NopMetrics — пустая реализация Metrics. Используется по умолчанию и в тестах,
// чтобы не тащить реестр Prometheus.
var NopMetrics Metrics = nopMetrics{}

type nopMetrics struct{}

func (nopMetrics) Hit(string)                                   {}
func (nopMetrics) Miss(string)                                  {}
func (nopMetrics) Error(string)                                 {}
func (nopMetrics) ObserveLatency(string, string, time.Duration) {}
