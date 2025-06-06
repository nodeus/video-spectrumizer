package main

import (

	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"gopkg.in/ini.v1"
)

var (
	Version	= "dev"
	BuildUser = "nd"
	BuildTime = "2025"
)

type Config struct {
	InputVideo    string  `ini:"input"`
	OutputVideo   string  `ini:"output"`
	TempDir       string  `ini:"temp"`
	Framerate     float64 `ini:"fps"`
	ScaleFactor   int     `ini:"scale"`
	AudioBitrate  string  `ini:"audio-bitrate"`
	EncoderType   string  `ini:"encoder"`
	ConfigFile    string  `ini:"config"`
	ImgConverter  string  `ini:"converter"`
	DeleteTemp    bool    `ini:"cleanup"`
	PauseBefore   bool    `ini:"pause"`
	Threads       int     `ini:"threads"`
	ResizeWidth   int     `ini:"width"`
	ResizeHeight  int     `ini:"height"`
	ShowFFmpegOut bool    `ini:"verbose-ffmpeg"`
	ShowProgress  bool    `ini:"progress"` // Новый параметр для отображения прогресса
}

const defaultConfigFile = "config.ini"

func main() {
	// Выводим версию приложения
	fmt.Println("Video spectrumizer", Version, "\n")
	//fmt.Println("Version:\t", Version)
	fmt.Println("Build time:", BuildTime)
	fmt.Println("Build user:", BuildUser)
	fmt.Println()
	
	// Предварительная обработка флага -config
	configFile := defaultConfigFile
	if len(os.Args) > 1 {
		for i, arg := range os.Args[1:] {
			if arg == "-config" && i+1 < len(os.Args)-1 {
				configFile = os.Args[i+2]
				break
			} else if strings.HasPrefix(arg, "-config=") {
				configFile = strings.SplitN(arg, "=", 2)[1]
				break
			}
		}
	}

	// Загрузка конфигурации
	config := loadDefaultConfig()
	if err := loadIniConfig(configFile, config); err != nil {
		log.Printf("Ошибка загрузки конфига: %v. Используются значения по умолчанию", err)
	}
	parseFlags(config)

	// Получаем абсолютные пути для всех компонентов
	config = resolvePaths(config)

	frameDir := filepath.Join(config.TempDir, "frames")
	processedDir := filepath.Join(config.TempDir, "processed")
	createDir(frameDir)
	createDir(processedDir)

	var hasAudio bool

    log.Println("Проверка наличия аудиодорожки...")
    hasAudio = checkAudioExists(config.InputVideo, config)
    fmt.Println()

    if hasAudio {
        log.Println("Извлечение аудиодорожки...")
        fmt.Println()
        extractAudio(config.InputVideo, filepath.Join(config.TempDir, "sound.wav"), config)
    } else {
        log.Println("Аудиодорожка не обнаружена, видео будет создано без звука")
        fmt.Println()
    }

	log.Println("Изменение размера видео...")
	fmt.Println()
	resizedVideo := filepath.Join(config.TempDir, "resized.mp4")
	resizeVideo(config.InputVideo, resizedVideo, config.ResizeWidth, config.ResizeHeight, config)

	log.Println("Разбивка видео на кадры...")
	fmt.Println()
	extractFrames(resizedVideo, frameDir, config.Framerate, config)

	if config.PauseBefore {
		fmt.Println("\nПауза для настройки конвертера. Нажмите Enter для продолжения...")
		fmt.Println()
		bufio.NewReader(os.Stdin).ReadBytes('\n')
	}

	log.Println("Обработка кадров для Spectrum конвертором...")
	fmt.Println()
	processFrames(frameDir, processedDir, config)

    log.Println("Сборка финального видео...")
    encodeVideo(
        filepath.Join(config.TempDir, "sound.wav"),
        hasAudio, // Передаем флаг наличия аудио
        processedDir,
        config.OutputVideo,
        config,
    )
    fmt.Println()

	if config.DeleteTemp {
		log.Println("Очистка временных файлов...")
		os.RemoveAll(config.TempDir)
		fmt.Println()
	}

	log.Println("Обработка видео завершена!")
}

// Функция для проверки наличия аудио необходимо наличие ffbrobe по пути
func checkAudioExists(input string, config *Config) bool {
    args := []string{
        "-i", input,
        "-show_entries", "stream=codec_type",
        "-of", "csv=p=0",
        "-loglevel", "error",
    }

    cmd := exec.Command("ffprobe", args...)
    output, err := cmd.CombinedOutput()
    if err != nil {
        log.Printf("Ошибка проверки аудио: %v", err)
        return false
    }

    return strings.Contains(string(output), "audio")
}

func loadDefaultConfig() *Config {
	return &Config{
		InputVideo:    "",
		OutputVideo:   "video-out.mp4",
		TempDir:       "temp",
		Framerate:     25.0,
		ScaleFactor:   8,
		AudioBitrate:  "384k",
		EncoderType:   "nvidia",
		ConfigFile:    "conv.isw",
		ImgConverter:  "img2spectrum.exe",
		DeleteTemp:    true,
		PauseBefore:   true,
		Threads:       runtime.NumCPU(),
		ResizeWidth:   256,
		ResizeHeight:  192,
		ShowFFmpegOut: true,
		ShowProgress:  true, // По умолчанию показываем прогресс
	}
}

func loadIniConfig(filename string, config *Config) error {
	iniFile, err := ini.Load(filename)
	if err != nil {
		return err
	}

	// Загружаем только секцию [default]
	section := iniFile.Section("default")
	if err = section.MapTo(config); err != nil {
		return err
	}

	return nil
}

func parseFlags(config *Config) {
	flag.StringVar(&config.InputVideo, "input", config.InputVideo, "Входной видеофайл (обязательно)")
	flag.StringVar(&config.OutputVideo, "output", config.OutputVideo, "Выходной видеофайл")
	flag.StringVar(&config.TempDir, "temp", config.TempDir, "Директория для временных файлов")
	flag.Float64Var(&config.Framerate, "fps", config.Framerate, "Частота кадров")
	flag.IntVar(&config.ScaleFactor, "scale", config.ScaleFactor, "Масштаб увеличения")
	flag.StringVar(&config.AudioBitrate, "audio-bitrate", config.AudioBitrate, "Битрейт аудио")
	flag.StringVar(&config.EncoderType, "encoder", config.EncoderType, "Тип кодировщика (cpu/nvidia/amd)")
	flag.StringVar(&config.ConfigFile, "config", config.ConfigFile, "Конфиг для img2spectrum")
	flag.StringVar(&config.ImgConverter, "converter", config.ImgConverter, "Путь к конвертеру")
	flag.BoolVar(&config.DeleteTemp, "cleanup", config.DeleteTemp, "Удалять временные файлы")
	flag.BoolVar(&config.PauseBefore, "pause", config.PauseBefore, "Пауза перед конвертацией")
	flag.IntVar(&config.Threads, "threads", config.Threads, "Количество потоков")
	flag.IntVar(&config.ResizeWidth, "width", config.ResizeWidth, "Ширина после ресайза")
	flag.IntVar(&config.ResizeHeight, "height", config.ResizeHeight, "Высота после ресайза")
	flag.BoolVar(&config.ShowFFmpegOut, "verbose-ffmpeg", config.ShowFFmpegOut, "Показывать вывод FFmpeg")
	flag.BoolVar(&config.ShowProgress, "progress", config.ShowProgress, "Показывать прогресс обработки")

	// Специальный флаг для указания конфиг-файла (не сохраняется в структуре)
	var configFile string
	flag.StringVar(&configFile, "config-file", defaultConfigFile, "Путь к INI-конфигу")

	flag.Parse()
}

// Преобразование путей в абсолютные
func resolvePaths(config *Config) *Config {
	absPath := func(path string) string {
		if path == "" {
			return path
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			log.Printf("Ошибка преобразования пути %s: %v", path, err)
			return path
		}
		return abs
	}

	config.InputVideo = absPath(config.InputVideo)
	config.OutputVideo = absPath(config.OutputVideo)
	config.TempDir = absPath(config.TempDir)
	config.ConfigFile = absPath(config.ConfigFile)
	config.ImgConverter = absPath(config.ImgConverter)

	return config
}

func validateConfig(config *Config) {
	if config.InputVideo == "" {
		log.Fatal("Ошибка: Не указан входной файл")
	}
	if config.Threads < 1 {
		config.Threads = runtime.NumCPU()
	}

	// Проверка существования img2spectrum
	if _, err := os.Stat(config.ImgConverter); os.IsNotExist(err) {
		log.Fatalf("Конвертер не найден: %s", config.ImgConverter)
	}
}

func createDir(path string) {
	if err := os.MkdirAll(path, os.ModePerm); err != nil {
		log.Fatalf("Ошибка создания директории %s: %v", path, err)
	}
}

// Извлечение аудио с возвратом ошибки
func extractAudio(input, output string, config *Config) error {
    args := []string{
        "-loglevel", "error",
        "-i", input,
        "-vn",
        "-acodec", "pcm_s16le",
        "-ar", "44100",
        "-ac", "2",
        "-y",
        output,
    }
    return runCommand("ffmpeg", args, config)
}

// -vf scale заменена на пробную универсальную функцию масштабирования
func resizeVideo(input, output string, width, height int, config *Config) {
	args := []string{
		"-loglevel", "error",
		"-i", input,
		"-vf", fmt.Sprintf("scale=w=%d:h=%d:force_original_aspect_ratio=decrease, pad=%d:%d:(%d-iw)/2:(%d-ih)/2:color=black", width, height, width, height, width, height),
		"-c:a", "copy",
		"-y",
		output,
	}
	runCommand("ffmpeg", args, config)
}

func extractFrames(input, outputDir string, framerate float64, config *Config) {
	// Создаем шаблон с правильным разделителем для FFmpeg
	pattern := filepath.Join(outputDir, "%06d.png")
	pattern = strings.ReplaceAll(pattern, "\\", "/") // FFmpeg требует / даже в Windows

	args := []string{
		"-loglevel", "error",
		"-i", input,
		"-vf", "fps=" + fmt.Sprint(framerate),
		"-y",
		pattern,
	}
	runCommand("ffmpeg", args, config)
}

func processFrames(inputDir, outputDir string, config *Config) {
	files, err := filepath.Glob(filepath.Join(inputDir, "*.png"))
	if err != nil {
		log.Fatalf("Ошибка поиска файлов: %v", err)
	}

	totalFrames := len(files)
	if totalFrames == 0 {
		log.Fatal("Не найдены кадры для обработки")
	}

	log.Printf("Начата обработка %d кадров...", totalFrames)
	startTime := time.Now()
	fmt.Println()

	// Счетчик для отслеживания прогресса
	var processedCount int
	var progressMutex sync.Mutex

	// Канал для ошибок
	errorChan := make(chan error, len(files))

	// Запускаем горутину для отображения прогресса
	progressQuit := make(chan struct{})
	if config.ShowProgress {
		go func() {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					progressMutex.Lock()
					current := processedCount
					progressMutex.Unlock()

					percent := float64(current) / float64(totalFrames) * 100
					elapsed := time.Since(startTime).Round(time.Second)

					fmt.Printf("\rПрогресс: %d/%d (%.1f%%) | Время: %v  ",
						current, totalFrames, percent, elapsed)

				case <-progressQuit:
					fmt.Println()
					return
				}
			}
		}()
	}

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, config.Threads)

	for _, file := range files {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(inputFile string) {
			defer wg.Done()
			defer func() { <-semaphore }()

			baseName := filepath.Base(inputFile)
			outputFile := filepath.Join(outputDir, "s"+baseName)

			cmd := exec.Command(
				config.ImgConverter,
				inputFile,
				config.ConfigFile,
				"-p", outputFile,
			)

			if output, err := cmd.CombinedOutput(); err != nil {
				errorChan <- fmt.Errorf("ошибка обработки %s: %v\n%s", inputFile, err, string(output))
			} else {
				// Увеличиваем счетчик обработанных кадров
				progressMutex.Lock()
				processedCount++
				progressMutex.Unlock()
			}
		}(file)
	}

	wg.Wait()

	// Останавливаем отображение прогресса
	if config.ShowProgress {
		close(progressQuit)
	}

	close(errorChan)

	// Выводим ошибки, если они есть
	hasErrors := false
	for err := range errorChan {
		log.Println(err)
		hasErrors = true
	}

	// Финальный отчет
	elapsed := time.Since(startTime).Round(time.Second)
	fps := float64(totalFrames) / time.Since(startTime).Seconds()

	log.Printf("\nОбработка завершена: %d/%d кадров | Затрачено: %v | Скорость: %.1f fps",
		processedCount, totalFrames, elapsed, fps)
	fmt.Println()

	if hasErrors {
		log.Println("\nБыли ошибки при обработке некоторых кадров")
	}
}

func encodeVideo(audioFile string, hasAudio bool, framesDir, output string, config *Config) {
    framePattern := filepath.Join(framesDir, "s%06d.png")
    framePattern = strings.ReplaceAll(framePattern, "\\", "/")

    args := []string{
        "-loglevel", "panic",
        "-y",
        "-framerate", fmt.Sprintf("%.2f", config.Framerate),
        "-i", framePattern,
        "-vf", fmt.Sprintf("scale=iw*%d:ih*%d", config.ScaleFactor, config.ScaleFactor),
        "-sws_flags", "neighbor",
        "-sws_dither", "none",
    }

	// Добавляем аудио только если оно есть
    if hasAudio {
        args = append([]string{"-i", audioFile}, args...)
        args = append(args,
            "-c:a", "aac",
            "-b:a", config.AudioBitrate,
            "-profile:a", "aac_low",
        )
    } else {
        // Явно указываем отсутствие аудио
        args = append(args, "-an")
    }

    // Общие параметры
    args = append(args,
        "-movflags", "+faststart",
        "-flags", "+cgop",
    )

	switch strings.ToLower(config.EncoderType) {
	case "nvidia":
		args = append(args,
			"-c:v", "hevc_nvenc",
			"-profile:v", "main10",
			"-pix_fmt", "yuv420p",
			"-preset", "fast",
			"-rc", "constqp",
			"-qp", "17",
			"-init_qpB", "2",
		)
	case "amd":
		args = append(args,
			"-c:v", "hevc_amf",
			"-rc", "cqp",
			"-qp_p", "17",
			"-qp_i", "17",
			"-pix_fmt", "yuv420p",
		)
	default: // CPU
		args = append(args,
			"-c:v", "libx264",
			"-crf", "17",
			"-pix_fmt", "yuv420p",
		)
	}

	args = append(args, output)
	runCommand("ffmpeg", args, config)
}

func runCommand(name string, args []string, config *Config) error {
    cmd := exec.Command(name, args...)

    if config.ShowFFmpegOut {
        cmd.Stdout = os.Stdout
        cmd.Stderr = os.Stderr
        log.Printf("Выполнение: %s %s", name, strings.Join(args, " "))
        fmt.Println()
    } else {
        cmd.Stdout = nil
        cmd.Stderr = nil
    }

    if err := cmd.Run(); err != nil {
        return fmt.Errorf("ошибка выполнения команды:\n%s %s\n%v", name, strings.Join(args, " "), err)
    }
    return nil
}
