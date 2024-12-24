package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

const (
	maxConcurrency       = 2  // Máximo número de conexiones concurrentes
	requestDelay         = 3  // Retardo entre solicitudes en segundos
	outputDir            = "screens"
	requestTimeout       = 15 * time.Second // Tiempo límite para solicitudes HTTP
	screenshotTimeout    = 20 * time.Second // Tiempo límite para capturas de pantalla
	networkPauseDuration = 1 * time.Second  // Pausa entre lotes de dominios
)

type ScreenshotData struct {
	Domain    string
	ImagePath string
}

func main() {
	fmt.Println("Iniciando verificación de directorios .git...")

	if len(os.Args) < 2 {
		fmt.Println("Uso: go run main.go <archivo.txt>")
		return
	}

	filePath := os.Args[1]
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Printf("Error al abrir el archivo: %v\n", err)
		return
	}
	defer file.Close()

	if _, err := os.Stat(outputDir); os.IsNotExist(err) {
		os.Mkdir(outputDir, os.ModePerm)
	}

	var screenshotData []ScreenshotData

	jobs := make(chan string)
	var wg sync.WaitGroup

	for i := 0; i < maxConcurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for domain := range jobs {
				processDomain(domain, &screenshotData)
				time.Sleep(requestDelay * time.Second)
			}
		}()
	}

	scanner := bufio.NewScanner(file)
	batchSize := 10
	domainBatch := make([]string, 0, batchSize)
	for scanner.Scan() {
		domain := strings.TrimSpace(scanner.Text())
		if domain != "" {
			domainBatch = append(domainBatch, domain)
			if len(domainBatch) >= batchSize {
				for _, d := range domainBatch {
					jobs <- d
				}
				domainBatch = domainBatch[:0]
				time.Sleep(networkPauseDuration)
			}
		}
	}

	if len(domainBatch) > 0 {
		for _, d := range domainBatch {
			jobs <- d
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("Error al leer el archivo: %v\n", err)
	}

	close(jobs)
	wg.Wait()

	fmt.Println("Verificación completada. Resultados guardados en capturas de pantalla.")
}

func processDomain(domain string, screenshotData *[]ScreenshotData) {
	url := fmt.Sprintf("%s/.git/", domain)
	if checkGitDirectory(url) {
		if hasGitIndexTitle(url) {
			imagePath := captureScreenshot(domain)
			if imagePath != "" {
				*screenshotData = append(*screenshotData, ScreenshotData{
					Domain:    domain,
					ImagePath: imagePath,
				})
			}
		}
	}
}

func checkGitDirectory(url string) bool {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{
		Timeout:   requestTimeout,
		Transport: transport,
	}

	resp, err := client.Get(url)
	if err != nil {
		fmt.Printf("[ERROR] No se pudo conectar a %s: %v\n", url, err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		fmt.Printf("[OK] Directorio .git encontrado: %s\n", url)
		return true
	}

	fmt.Printf("[FAIL] No se encontró el directorio .git en: %s (Status: %d)\n", url, resp.StatusCode)
	return false
}

func hasGitIndexTitle(url string) bool {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{
		Timeout:   requestTimeout,
		Transport: transport,
	}

	resp, err := client.Get(url)
	if err != nil {
		fmt.Printf("[ERROR] No se pudo conectar a %s: %v\n", url, err)
		return false
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("[ERROR] No se pudo leer el cuerpo de la respuesta de %s: %v\n", url, err)
		return false
	}

	if strings.Contains(string(body), "<title>Index of /.git</title>") {
		fmt.Printf("[MATCH] Título 'Index of /.git' encontrado en %s\n", url)
		return true
	}

	fmt.Printf("[NO MATCH] Título 'Index of /.git' no encontrado en %s\n", url)
	return false
}

func captureScreenshot(domain string) string {
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, screenshotTimeout)
	defer cancel()

	imagePath := filepath.Join(outputDir, fmt.Sprintf("%s.png", sanitizeFilename(domain)))

	var buf []byte
	err := chromedp.Run(ctx, chromedp.Tasks{
		chromedp.EmulateViewport(800, 600),
		chromedp.Navigate(domain),
		chromedp.WaitVisible(`body`, chromedp.ByQuery),
		chromedp.Screenshot(`body`, &buf, chromedp.NodeVisible),
	})
	if err != nil {
		fmt.Printf("[ERROR] No se pudo capturar la pantalla de %s: %v\n", domain, err)
		return ""
	}

	err = ioutil.WriteFile(imagePath, buf, 0644)
	if err != nil {
		fmt.Printf("[ERROR] No se pudo guardar la captura de pantalla de %s: %v\n", domain, err)
		return ""
	}

	fmt.Printf("[SCREENSHOT] Captura guardada: %s\n", imagePath)
	return imagePath
}

func sanitizeFilename(name string) string {
	return strings.ReplaceAll(name, "://", "_")
}

