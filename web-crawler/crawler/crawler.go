package crawler

import (
	"bufio"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Result представляет структуру успешно обработанной страницы.
// Result represents the data structure of a successfully processed page.
type Result struct {
	URL   string // Оригинальный адрес / Source URL
	Title string // Заголовок страницы (упрощенно) / Page identifier or preview stub
}

// WebCrawler координирует распределенный асинхронный обход веб-страниц.
// WebCrawler coordinates the concurrent asynchronous scraping of web pages.
type WebCrawler struct {
	mu           sync.Mutex
	visited      map[string]bool // Хранилище уникальности (Дерепликация) / De-duplication visited cache
	hostRegistry map[string]bool // Реестр вежливости по хостам / Politeness host activity tracker
	frontier     chan string     // Очередь URL Frontier / Task distribution pathway (Frontier)
	results      chan Result     // Канал результатов для клиента / Output pipeline for processed data
	workerCount  int             // Размер пула воркеров / Worker pool capacity constraint
	wg           sync.WaitGroup  // Синхронизатор завершения работы / Completion state synchronizer
	httpClient   *http.Client    // Указатель на синглтон HTTP-клиента для переиспользования TCP-пула / Singleton HTTP client pointer for TCP pool reuse
	linkRegexp   *regexp.Regexp  // Переиспользуемое скомпилированное регулярное выражение / Reusable pre-compiled regex layout
}

// NewWebCrawler инициализирует потокобезопасный пул воркеров краулера со сквозным HTTP-клиентом.
// NewWebCrawler initializes a thread-safe crawler worker pool instance with a reusable HTTP client.
func NewWebCrawler(workerCount, bufferSize int) *WebCrawler {
	return &WebCrawler{
		visited:      make(map[string]bool),
		hostRegistry: make(map[string]bool),
		frontier:     make(chan string, bufferSize),
		results:      make(chan Result, bufferSize),
		workerCount:  workerCount,
		// Инициализируем один клиент на все воркеры с жестким таймаутом на I/O операции
		// Initialize a single client across all workers with a strict I/O timeout constraint
		httpClient: &http.Client{
			Timeout: 3 * time.Second,
		},
		// Компилируем регулярное выражение ровно ОДИН раз при создании краулера
		// Compile the regular expression exactly ONCE upon crawler architecture initialization
		linkRegexp: regexp.MustCompile(`href="(https?://[^"]+)"`),
	}
}

// Start запускает воркеры в бэкграунде и закладывает стартовые URL (Seed).
// Start provisions workers into the background and seeds the initial frontier.
func (c *WebCrawler) Start(seeds []string) <-chan Result {
	// Запускаем фиксированный пул горутин-воркеров
	// Launch a fixed pool of execution goroutine workers
	for i := 0; i < c.workerCount; i++ {
		c.wg.Add(1)
		go c.workerLoop()
	}

	// Подаем стартовые URL (Seed URLs) в URL Frontier
	// Feed seed URLs into the URL Frontier pipeline
	for _, seed := range seeds {
		c.frontier <- seed
	}

	// Горутина-наблюдатель для закрытия каналов после завершения всех задач
	// Background coordinator to shut down streams once tasks deplete
	go func() {
		c.wg.Wait()
		close(c.results)
	}()

	return c.results
}

// Stop принудительно останавливает URL Frontier
// Stop abruptly terminates the URL Frontier pipeline
func (c *WebCrawler) Stop() {
	close(c.frontier)
}

// workerLoop представляет бесконечный цикл обработки задач горутиной.
// workerLoop defines the infinite task execution lifecycle for a worker thread.
func (c *WebCrawler) workerLoop() {
	defer c.wg.Done()

	// Воркеры пассивно читают из канала Frontier. Если канал закрыт, цикл завершится.
	// Workers read from Frontier. If the stream closes, the loop terminates smoothly.
	for targetURL := range c.frontier {
		if !c.shouldCrawl(targetURL) {
			continue
		}

		// Выполняем HTTP-запрос и парсинг HTML
		// Execute outbound HTTP transaction and HTML parsing
		res, links, err := c.fetchAndParse(targetURL)
		if err != nil {
			c.releaseHost(targetURL)
			continue
		}

		// Отправляем успешный результат клиенту
		// Dispatch the successfully parsed record to the output channel
		c.results <- res

		// Освобождаем хост и лениво добавляем новые найденные ссылки во Frontier
		// Release host lock and reactively append newly discovered edges to the Frontier
		c.releaseHost(targetURL)
		for _, link := range links {
			select {
			case c.frontier <- link:
			default:
				// Очередь Frontier переполнена, отбрасываем ссылку для защиты памяти
				// Frontier capacity saturated, drop the link to safeguard memory boundaries
			}
		}
	}
}

// shouldCrawl реализует логику Дерепликации и Вежливости (Politeness Boundary).
// shouldCrawl executes concurrent De-duplication and Politeness checking filters.
func (c *WebCrawler) shouldCrawl(targetURL string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	u, err := url.Parse(targetURL)
	if err != nil {
		return false
	}

	host := u.Host

	// 1. ДЕРЕПЛИКАЦИЯ: Если мы уже скачивали этот URL — игнорируем
	// 1. DE-DUPLICATION: If the URL has been encountered before — drop it
	if c.visited[targetURL] {
		return false
	}

	// 2. АРХИТЕКТУРА ВЕЖЛИВОСТИ: Если другая горутина прямо сейчас качает этот хост — пропускаем,
	// чтобы не устроить непреднамеренную DDoS-атаку на чужой сервер.
	// 2. POLITENESS BOUNDARY: If another thread is actively fetching this host — skip,
	// preventing accidental DDoS vectors on downstream production infrastructure.
	if c.hostRegistry[host] {
		return false
	}

	// Задаем блокировку хоста и помечаем URL как посещенный
	// Lock the host scope and mark the URL target as processed
	c.hostRegistry[host] = true
	c.visited[targetURL] = true
	return true
}

// releaseHost снимает блокировку вежливости с хоста.
// releaseHost clears the politeness activity flag from the host token.
func (c *WebCrawler) releaseHost(targetURL string) {
	u, err := url.Parse(targetURL)
	if err != nil {
		return
	}

	c.mu.Lock()
	delete(c.hostRegistry, u.Host)
	c.mu.Unlock()
}

// fetchAndParse скачивает страницу и извлекает ссылки через регулярные выражения.
// fetchAndParse downloads the body content and extracts raw links via regex filters.
func (c *WebCrawler) fetchAndParse(targetURL string) (Result, []string, error) {
	// ИСПОЛЬЗУЕМ СИНГЛТОН-КЛИЕНТ: Переиспользуем TCP-соединения из пула структуры
	// UTILIZING SINGLETON CLIENT: Reuse active TCP sockets from the structural connection pool
	resp, err := c.httpClient.Get(targetURL)
	if err != nil {
		return Result{}, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Result{}, nil, fmt.Errorf("bad status: %d", resp.StatusCode)
	}

	// Эффективный потоковый парсинг через bufio, чтобы не аллоцировать гигабайты под огромные HTML
	// Efficient stream parsing via bufio to block massive allocation spikes on giant HTML bodies
	var (
		links   []string
		matches [][]string

		titleDetected string
	)

	scanner := bufio.NewScanner(resp.Body)

	for scanner.Scan() {
		line := scanner.Text()

		// Попутно ищем тег заголовка, если он попадется
		// Try to capture basic title boundaries if encountered in the stream
		if strings.Contains(line, "<title>") && titleDetected == "" {
			titleDetected = line
		}

		matches = c.linkRegexp.FindAllStringSubmatch(line, -1)
		for matchId := range matches {
			if len(matches[matchId]) > 1 {
				links = append(links, matches[matchId][1])
			}
		}
	}

	if titleDetected == "" {
		titleDetected = "Парсинг завершен"
	}

	res := Result{
		URL:   targetURL,
		Title: strings.TrimSpace(titleDetected),
	}

	return res, links, nil
}
