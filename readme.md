# video spectrumizer
Программа для конвертирования видео в стилизованное под zx spectrum.

### Использование программы:
#### Базовое использование
video-spectrumizer.exe -input video-input.mp4

video-spectrumizer.exe -input video-input.mp4 -output video-out.mp4

video-spectrumizer.exe -input video-input.mp4 -output video-out.mp4 -scale 8

#### Расширенный пример
video-spectrumizer.exe \
  -input video-input.mp4 \
  -output video-out.mp4 \
  -encoder nvidia \
  -fps 30 \
  -scale 16 \
  -threads 8 \
  -width 256 \
  -height 192 \
  -pause true \
  -cleanup false

### Параметры командной строки:

| Параметр         | По умолчанию     | Описание                             |
| ---------------- | ---------------- | ------------------------------------ |
| `-input`         | (обязательно)    | Входной видеофайл                    |
| `-output`        | video-out.mp4    | Выходной файл                        |
| `-temp`          | temp             | Директория для временных файлов      |
| `-fps`           | 25.0             | Частота кадров                       |
| `-scale`         | 8                | Масштаб увеличения выходного видео   |
| `-audio-bitrate` | 384k             | Битрейт аудио для AAC                |
| `-encoder`       | cpu              | Кодировщик (cpu/nvidia/amd)          |
| `-config`        | conv.isw         | Конфиг для img2spectrum              |
| `-converter`     | img2spectrum.exe | Путь к конвертеру                    |
| `-cleanup`       | true             | Удалять временные файлы              |
| `-pause`         | false            | Пауза перед конвертацией             |
| `-threads`       | 8                | Количество потоков обработки         |
| `-width`         | 256              | Ширина после ресайза                 |
| `-height`        | 192              | Высота после ресайза                 |
| `-progress`      | true             | Отображение прогресса конвертирования|
| `-scr`           | false            | Сохранение .scr файлов               |

### Требования:
1. Установленный в системе FFmpeg и ffprobe (доступный через PATH)
2. Установленный в системе img2spectrum.exe (доступный через путь в конфиге или в папке проекта)

[Скачать](https://github.com/nodeus/video-spectrumizer/releases)
