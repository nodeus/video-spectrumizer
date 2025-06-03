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
	ShowFFmpegOut bool
}

func main() {
	config := parseFlags()
	validateConfig(config)

	// Получаем абсолютные пути для всех компонентов
	config = resolvePaths(config)

	frameDir := filepath.Join(config.TempDir, "frames")
	processedDir := filepath.Join(config.TempDir, "processed")
	createDir(frameDir)
	createDir(processedDir)

	log.Println("Извлечение аудиодорожки...")
	extractAudio(config.InputVideo, filepath.Join(config.TempDir, "sound.wav"), config)

	log.Println("Изменение размера видео...")
	resizedVideo := filepath.Join(config.TempDir, "resized.mp4")
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
	flag.IntVar(&config.ScaleFactor, "scale", 8, "Масштаб увеличения")
	flag.StringVar(&config.AudioBitrate, "audio-bitrate", "384k", "Битрейт аудио")
	flag.StringVar(&config.EncoderType, "encoder", "nvidia", "Тип кодировщика (cpu/nvidia/amd)")
	flag.StringVar(&config.ConfigFile, "config", "conv.isw", "Конфиг для img2spectrum")
	flag.StringVar(&config.ImgConverter, "converter", "f:/Portable/img2spec/img2spectrum.exe", "Путь к конвертеру")
	flag.BoolVar(&config.DeleteTemp, "cleanup", true, "Удалять временные файлы")
	flag.BoolVar(&config.PauseBefore, "pause", true, "Пауза перед конвертацией")
	flag.IntVar(&config.Threads, "threads", runtime.NumCPU(), "Количество потоков")
	flag.IntVar(&config.ResizeWidth, "width", 256, "Ширина после ресайза")
	flag.IntVar(&config.ResizeHeight, "height", -1, "Высота после ресайза")
	flag.BoolVar(&config.ShowFFmpegOut, "verbose-ffmpeg", true, "Показывать вывод FFmpeg")

	flag.Parse()
	return config
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

func extractAudio(input, output string, config *Config) {
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
	runCommand("ffmpeg", args, config)
}

func resizeVideo(input, output string, width, height int, config *Config) {
	args := []string{
		"-loglevel", "error",
		"-i", input,
		"-vf", fmt.Sprintf("scale=%d:%d", width, height),
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

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, config.Threads)
	errorChan := make(chan error, len(files))

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
			}
		}(file)
	}

	wg.Wait()
	close(errorChan)

	for err := range errorChan {
		log.Println(err)
	}
}

func encodeVideo(audioFile, framesDir, output string, config *Config) {
	// Создаем шаблон с правильным разделителем
	framePattern := filepath.Join(framesDir, "s%06d.png")
	framePattern = strings.ReplaceAll(framePattern, "\\", "/") // FFmpeg требует / даже в Windows

	args := []string{
		"-loglevel", "error",
		"-y",
		"-i", audioFile,
		"-framerate", fmt.Sprintf("%.2f", config.Framerate),
		"-i", framePattern,
		"-vf", fmt.Sprintf("scale=iw*%d:ih*%d", config.ScaleFactor, config.ScaleFactor),
		"-sws_flags", "neighbor",
		"-sws_dither", "none",
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

func runCommand(name string, args []string, config *Config) {
	cmd := exec.Command(name, args...)

	if config.ShowFFmpegOut {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		log.Printf("Выполнение: %s %s", name, strings.Join(args, " "))
	} else {
		cmd.Stdout = nil
		cmd.Stderr = nil
	}

	if err := cmd.Run(); err != nil {
		log.Fatalf("Ошибка выполнения команды:\n%s %s\n%v", name, strings.Join(args, " "), err)
	}
}
