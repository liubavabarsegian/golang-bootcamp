package main

import (
	"context"
	"flag"
	"log"
	"math"
	"time"

	pb "APG1-Bootcamp/Go_Team00-2/src/task-02-report/pkg/frequency"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const (
	defaultAddress = "localhost:50051"                                                                          // Адрес сервера по умолчанию
	defaultK       = 2.0                                                                                        // Значение коэффициента обнаружения аномалий по умолчанию
	dsn            = "host=localhost user=postgres dbname=postgres password=postgres port=5432 sslmode=disable" // DSN для подключения к PostgreSQL
)

// Anomaly представляет структуру для записи аномалий в базе данных
type Anomaly struct {
	gorm.Model
	Frequency  float64
	Mean       float64
	StdDev     float64
	DetectedAt time.Time
}

func main() {
	// Параметры командной строки
	address := flag.String("address", defaultAddress, "Server address") // Адрес сервера, который будет использоваться
	k := flag.Float64("k", defaultK, "Anomaly detection coefficient")   // Коэффициент для определения аномалий
	flag.Parse()                                                        // Обработка параметров командной строки

	// Создание контекста
	ctx := context.Background() // Контекст для gRPC операций

	// Создание gRPC клиента с использованием новых методов
	conn, err := grpc.NewClient(*address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect: %v", err) // Логирование ошибки, если соединение не удалось установить
	}
	defer conn.Close() // Закрытие соединения при завершении работы функции main

	client := pb.NewFrequencyServiceClient(conn) // Создание нового клиента gRPC для общения с сервисом частот

	// Подключение к базе данных PostgreSQL
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err) // Логирование ошибки, если не удалось подключиться к базе данных
	}

	// Автоматическое миграция структуры Anomaly в базе данных
	if err := db.AutoMigrate(&Anomaly{}); err != nil {
		log.Fatalf("Failed to migrate database schema: %v", err) // Логирование ошибки, если не удалось выполнить миграцию схемы базы данных
	}

	// Вызов метода StreamFrequencies
	stream, err := client.StreamFrequencies(ctx, &pb.Empty{})
	if err != nil {
		log.Fatalf("Failed to start streaming: %v", err) // Логирование ошибки, если не удалось начать стриминг
	}

	var sum float64    // Сумма всех частот для вычисления среднего значения
	var sumSq float64  // Сумма квадратов частот для вычисления дисперсии
	var count int      // Счетчик полученных сообщений
	var mean float64   // Среднее значение частот
	var stdDev float64 // Стандартное отклонение частот

	for {
		msg, err := stream.Recv() // Получение сообщения из потока
		if err != nil {
			log.Fatalf("Stream error: %v", err) // Логирование ошибки, если возникла проблема с чтением потока
		}

		count++                         // Увеличение счетчика сообщений
		frequency := msg.GetFrequency() // Извлечение частоты из сообщения
		sum += frequency                // Обновление суммы частот
		sumSq += frequency * frequency  // Обновление суммы квадратов частот

		// Рассчитываем среднее и стандартное отклонение
		mean = sum / float64(count)                          // Вычисление среднего значения частот
		variance := (sumSq / float64(count)) - (mean * mean) // Вычисление дисперсии
		if variance < 0 {
			variance = 0 // Гарантируем, что дисперсия не отрицательная
		}
		stdDev = math.Sqrt(variance) // Вычисление стандартного отклонения

		log.Printf("Received frequency: %f, Mean: %f, StdDev: %f", frequency, mean, stdDev) // Логирование полученных данных

		// Обнаружение аномалий
		if math.Abs(frequency-mean) > *k*stdDev { // Проверка, является ли частота аномалией
			log.Printf("Anomaly detected! Frequency: %f, Mean: %f, StdDev: %f", frequency, mean, stdDev) // Логирование информации об обнаруженной аномалии

			// Запись аномалии в базу данных
			anomaly := Anomaly{
				Frequency:  frequency,
				Mean:       mean,
				StdDev:     stdDev,
				DetectedAt: time.Now(),
			}
			if err := db.Create(&anomaly).Error; err != nil {
				log.Printf("Failed to record anomaly: %v", err) // Логирование ошибки, если не удалось записать аномалию в базу данных
			}
		}
	}
}
