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
)

type Config struct {
	InputVideo    string
	OutputVideo   string
	TempDir       string
	Framerate     float64
	ScaleFactor   int
	AudioBitrate  string
	EncoderType   string
	ConfigFile    string
	ImgConverter  string
	DeleteTemp    bool
	PauseBefore   bool
	Threads       int
	ResizeWidth   int
	ResizeHeight  int
	ShowFFmpegOut bool // Новый флаг для контроля вывода FFmpeg
}

func main() {
	config := parseFlags()
	validateConfig(config)

	frameDir := filepath.Join(config.TempDir, "frames")
	processedDir := filepath.Join(config.TempDir, "processed")
	createDir(frameDir)
	createDir(processedDir)

	log.Println("Извлечение аудиодорожки...")
	extractAudio(config.InputVideo, filepath.Join(config.TempDir, "sound.wav"), config)

	log.Println("Изменение размера видео...")
	resizedVideo := filepath.Join(config.TempDir, "resized.webm")
	resizeVideo(config.InputVideo, resizedVideo, config.ResizeWidth, config.ResizeHeight, config)

	log.Println("Разбивка видео на кадры...")
	extractFrames(resizedVideo, frameDir, config.Framerate, config)

	if config.PauseBefore {
		fmt.Println("\nПауза для настройки конвертера. Нажмите Enter для продолжения...")
		bufio.NewReader(os.Stdin).ReadBytes('\n')
	}

	log.Println("Обработка кадров для Spectrum...")
	processFrames(frameDir, processedDir, config)

	log.Println("Сборка финального видео...")
	encodeVideo(
		filepath.Join(config.TempDir, "sound.wav"),
		processedDir,
		config.OutputVideo,
		config,
	)

	if config.DeleteTemp {
		log.Println("Очистка временных файлов...")
		os.RemoveAll(config.TempDir)
	}

	log.Println("Обработка завершена!")
}

func parseFlags() *Config {
	config := &Config{}

	flag.StringVar(&config.InputVideo, "input", "", "Входной видеофайл (обязательно)")
	flag.StringVar(&config.OutputVideo, "output", "video-out.mp4", "Выходной видеофайл")
	flag.StringVar(&config.TempDir, "temp", "temp", "Директория для временных файлов")
	flag.Float64Var(&config.Framerate, "fps", 25.0, "Частота кадров")
	flag.IntVar(&config.ScaleFactor, "scale", 16, "Масштаб увеличения")
	flag.StringVar(&config.AudioBitrate, "audio-bitrate", "384k", "Битрейт аудио")
	flag.StringVar(&config.EncoderType, "encoder", "cpu", "Тип кодировщика (cpu/nvidia/amd)")
	flag.StringVar(&config.ConfigFile, "config", "conv.isw", "Конфиг для img2spectrum")
	flag.StringVar(&config.ImgConverter, "converter", "img2spectrum.exe", "Путь к конвертеру")
	flag.BoolVar(&config.DeleteTemp, "cleanup", true, "Удалять временные файлы")
	flag.BoolVar(&config.PauseBefore, "pause", false, "Пауза перед конвертацией")
	flag.IntVar(&config.Threads, "threads", runtime.NumCPU(), "Количество потоков")
	flag.IntVar(&config.ResizeWidth, "width", 256, "Ширина после ресайза")
	flag.IntVar(&config.ResizeHeight, "height", 192, "Высота после ресайза")
	flag.BoolVar(&config.ShowFFmpegOut, "verbose-ffmpeg", false, "Показывать вывод FFmpeg")

	flag.Parse()
	return config
}

func validateConfig(config *Config) {
	if config.InputVideo == "" {
		log.Fatal("Ошибка: Не указан входной файл")
	}
	if config.Threads < 1 {
		config.Threads = runtime.NumCPU()
	}
}

func createDir(path string) {
	if err := os.MkdirAll(path, os.ModePerm); err != nil {
		log.Fatalf("Ошибка создания директории %s: %v", path, err)
	}
}

// Все функции FFmpeg теперь принимают конфиг для контроля вывода
func extractAudio(input, output string, config *Config) {
	args := []string{
		"-loglevel", "error", // Подавление стандартного вывода
		"-i", input,
		"-vn",
		"-acodec", "pcm_s16le",
		"-ar", "44100",
		"-ac", "2",
		"-y",
		output,
	}
	runCommand("ffmpeg", args, config)
}

func resizeVideo(input, output string, width, height int, config *Config) {
	args := []string{
		"-loglevel", "error", // Подавление стандартного вывода
		"-i", input,
		"-vf", fmt.Sprintf("scale=%d:%d", width, height),
		"-c:a", "copy",
		"-y",
		output,
	}
	runCommand("ffmpeg", args, config)
}

func extractFrames(input, outputDir string, framerate float64, config *Config) {
	pattern := filepath.Join(outputDir, "%06d.png")
	args := []string{
		"-loglevel", "error", // Подавление стандартного вывода
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

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, config.Threads)
	errorChan := make(chan error, len(files))

	for _, file := range files {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(inputFile string) {
			defer wg.Done()
			defer func() { <-semaphore }()

			outputFile := filepath.Join(
				outputDir,
				"s"+filepath.Base(inputFile),
			)

			cmd := exec.Command(
				config.ImgConverter,
				inputFile,
				config.ConfigFile,
				"-p", outputFile,
			)

			// Перенаправляем вывод конвертера в лог
			if output, err := cmd.CombinedOutput(); err != nil {
				errorChan <- fmt.Errorf("ошибка обработки %s: %v\n%s", inputFile, err, string(output))
			}
		}(file)
	}

	wg.Wait()
	close(errorChan)

	// Обработка ошибок
	for err := range errorChan {
		log.Println(err)
	}
}

func encodeVideo(audioFile, framesDir, output string, config *Config) {
	args := []string{
		"-loglevel", "error", // Подавление стандартного вывода
		"-y",
		"-i", audioFile,
		"-framerate", fmt.Sprintf("%.2f", config.Framerate),
		"-i", filepath.Join(framesDir, "s%06d.png"),
		"-vf", fmt.Sprintf("scale=iw*%d:ih*%d,sws_flags=neighbor", config.ScaleFactor, config.ScaleFactor),
		"-c:a", "aac",
		"-b:a", config.AudioBitrate,
		"-profile:a", "aac_low",
		"-movflags", "+faststart",
		"-flags", "+cgop",
	}

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

// Обновленная функция запуска команд
func runCommand(name string, args []string, config *Config) {
	cmd := exec.Command(name, args...)

	if config.ShowFFmpegOut {
		// Режим отладки: показываем весь вывод
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		log.Printf("Выполнение: %s %s", name, strings.Join(args, " "))
	} else {
		// Подавляем вывод, показываем только ошибки
		cmd.Stdout = nil
		cmd.Stderr = nil
	}

	if err := cmd.Run(); err != nil {
		// При ошибке показываем полную команду и вывод
		log.Fatalf("Ошибка выполнения команды:\n%s %s\n%v", name, strings.Join(args, " "), err)
	}
}
