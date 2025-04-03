package main

import (
        "bufio"
        "crypto/tls"
        "fmt"
        "io/ioutil"
        "net/http"
        "os"
        "strings"
        "sync"
        "time"
)

const (
        maxConcurrency       = 3  // Máximo número de conexiones concurrentes
        requestDelay         = 2  // Retardo entre solicitudes en segundos
        requestTimeout       = 7 * time.Second // Tiempo límite para solicitudes HTTP
        networkPauseDuration = 2 * time.Second  // Pausa entre lotes de dominios
        outputFilePath       = "findgitters.txt" // Archivo de salida para dominios válidos
)

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

        outputFile, err := os.Create(outputFilePath)
        if err != nil {
                fmt.Printf("Error al crear el archivo de salida: %v\n", err)
                return
        }
        defer outputFile.Close()

        outputMutex := &sync.Mutex{}

        jobs := make(chan string)
        var wg sync.WaitGroup

        for i := 0; i < maxConcurrency; i++ {
                wg.Add(1)
                go func() {
                        defer wg.Done()
                        for domain := range jobs {
                                processDomain(domain, outputFile, outputMutex)
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

        fmt.Println("Verificación completada. Resultados guardados en", outputFilePath)
}

func processDomain(domain string, outputFile *os.File, outputMutex *sync.Mutex) {
        url := fmt.Sprintf("%s/.git/", domain)
        if checkGitDirectory(url) {
                if hasGitIndexTitle(url) {
                        outputMutex.Lock()
                        fmt.Fprintf(outputFile, "%s\n", domain)
                        outputMutex.Unlock()
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

