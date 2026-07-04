package main

import (
	"fmt"
	"sort"
	"sync"
)

// Ограничение на количество возвращаемых подсказок в каждом узле
// Limit for the number of autocomplete suggestions stored in each node
const maxSuggestions = 5

// Suggestion описывает структуру одной поисковой подсказки
// Suggestion represents a single search suggestion structure
type Suggestion struct {
	Word      string // Само слово / The actual word
	Frequency int    // Частота запросов (популярность) / Search frequency (popularity)
}

// TrieNode представляет собой узел префиксного дерева
// TrieNode represents a node within the Trie structure
type TrieNode struct {
	// Дочерние узлы: гибрид дерева и хэш-таблицы для мгновенного перехода O(1)
	// Child nodes: a tree-map hybrid ensuring instant O(1) transitions
	Children map[rune]*TrieNode

	// Флаг, указывающий на окончание реального слова
	// Flag indicating if this node represents the end of a valid word
	IsEnd bool

	// Частота конкретно этого слова (если IsEnd == true)
	// Frequency of this exact word (meaningful only if IsEnd is true)
	Frequency int

	// Предрасчитанный кэш ТОП-5 подсказок, находящихся ниже данного узла
	// Precomputed cache of TOP-5 popular suggestions located beneath this node
	TopSuggestions []Suggestion
}

// NewTrieNode инициализирует новый узел дерева с выделением памяти под кэш
// NewTrieNode allocates and initializes a new Trie node with pre-allocated cache slice
func NewTrieNode() *TrieNode {
	return &TrieNode{
		Children:       make(map[rune]*TrieNode),
		TopSuggestions: make([]Suggestion, 0, maxSuggestions),
	}
}

// AutocompleteSystem инкапсулирует корень дерева и потокобезопасный мьютекс
// AutocompleteSystem encapsulates the Trie root and thread-safe RWMutex
type AutocompleteSystem struct {
	mu   sync.RWMutex
	root *TrieNode
}

// NewAutocompleteSystem создает и возвращает рабочую систему автозаполнения
// NewAutocompleteSystem constructs and returns a fully initialized autocomplete system
func NewAutocompleteSystem() *AutocompleteSystem {
	return &AutocompleteSystem{
		root: NewTrieNode(),
	}
}

// Insert добавляет или обновляет слово, каскадно пересчитывая кэши родителей снизу вверх
// Insert adds or updates a word, cascading cache updates upwards from leaf to root
func (as *AutocompleteSystem) Insert(word string, count int) {
	as.mu.Lock()
	defer as.mu.Unlock()

	// Срез для фиксации пройденного пути по узлам для последующего обновления кэша
	// Slice to track the exact path taken through nodes for reverse cache updating
	path := make([]*TrieNode, 0, len(word))
	current := as.root

	// Идем вглубь дерева по символам слова
	// Traverse deep into the Trie based on the characters of the word
	for _, char := range word {
		path = append(path, current)
		if _, exists := current.Children[char]; !exists {
			current.Children[char] = NewTrieNode()
		}
		current = current.Children[char]
	}

	// Фиксируем финальный узел слова и обновляем его метки
	// Register the final leaf node and update its metrics
	path = append(path, current)
	current.IsEnd = true
	current.Frequency += count

	// Формируем целевой объект подсказки для обновления в кэшах родителей
	// Construct the target suggestion object to update inside parent caches
	targetSuggestion := Suggestion{Word: word, Frequency: current.Frequency}

	// Каскадное обновление: поднимаемся обратно к корню и обновляем TopSuggestions
	// Cascading update: ascend back to root updating the TopSuggestions at each level
	for i := len(path) - 1; i >= 0; i-- {
		node := path[i]
		node.updateCache(targetSuggestion)
	}
}

// updateCache локально перестраивает отсортированный ТОП-5 кэш внутри узла
// updateCache locally recalculates and sorts the TOP-5 cache inside a single node
func (node *TrieNode) updateCache(s Suggestion) {
	foundIdx := -1
	// Ищем, присутствует ли уже данное слово в текущем кэше узла
	// Look if this specific word is already resident inside the node's cache
	for i, cached := range node.TopSuggestions {
		if cached.Word == s.Word {
			foundIdx = i
			break
		}
	}

	if foundIdx != -1 {
		// Если нашли — актуализируем частоту
		// If found, update its frequency in-place
		node.TopSuggestions[foundIdx].Frequency = s.Frequency
	} else {
		// Если нет — добавляем в срез
		// Otherwise, append the new suggestion into the slice
		node.TopSuggestions = append(node.TopSuggestions, s)
	}

	// Сортировка: сначала по популярности (desc), затем лексикографически (asc)
	// Sorting: higher frequency first (desc), alphabetic order upon tie (asc)
	sort.Slice(node.TopSuggestions, func(i, j int) bool {
		if node.TopSuggestions[i].Frequency == node.TopSuggestions[j].Frequency {
			return node.TopSuggestions[i].Word < node.TopSuggestions[j].Word
		}
		return node.TopSuggestions[i].Frequency > node.TopSuggestions[j].Frequency
	})

	// Усекаем слайс до лимита maxSuggestions (5), сохраняя контроль над памятью
	// Enforce the cache limit maxSuggestions (5) to maintain memory predictability
	if len(node.TopSuggestions) > maxSuggestions {
		node.TopSuggestions = node.TopSuggestions[:maxSuggestions]
	}
}

// Search возвращает список подсказок по префиксу за константное время O(L)
// Search evaluates and returns suggestions matching a prefix in true O(L) time
func (as *AutocompleteSystem) Search(prefix string) []string {
	as.mu.RLock()
	defer as.mu.RUnlock()

	current := as.root
	// Спускаемся по символам префикса
	// Travel downwards matching each character of the user's input prefix
	for _, char := range prefix {
		if next, exists := current.Children[char]; exists {
			current = next
		} else {
			return nil // Префикс отсутствует в системе / Prefix not found
		}
	}

	// Главный выигрыш Highload-архитектуры: нет рекурсии и обходов дерева на чтении!
	// Ultimate Highload benefit: absolute zero recursion or deep tree traversals on reads!
	result := make([]string, 0, len(current.TopSuggestions))
	for _, s := range current.TopSuggestions {
		result = append(result, s.Word)
	}

	return result
}

func main() {
	system := NewAutocompleteSystem()

	// Имитация пакетного наполнения данными (например, агрегация логов)
	// Mimicking batch data population (e.g., aggregate log processing pipelines)
	fmt.Println("=== Наполнение системы данными / Populating Data ===")
	system.Insert("go", 500)
	system.Insert("google", 300)
	system.Insert("golang", 100)
	system.Insert("github", 250)
	system.Insert("птица", 80)

	// Поиск по префиксу "go". Ожидаемый вывод: go (500), google (300), golang (100)
	// Querying prefix "go". Expected outcome: go (500), google (300), golang (100)
	fmt.Println("\n=== Поиск для префикса 'go' / Querying 'go' ===")
	for i, res := range system.Search("go") {
		fmt.Printf("%d. %s\n", i+1, res)
	}

	// Динамическое изменение популярности: "golang" резко вырывается вперед
	// Dynamic popularity surge: "golang" hits explosive usage growth
	fmt.Println("\n=== 'golang' набирает популярность (+450) / 'golang' popularity spikes ===")
	system.Insert("golang", 450) // 100 + 450 = 550

	// Повторный поиск. "golang" теперь занимает первую строчку (550 > 500)
	// Subsequent search execution. "golang" shifts to the top priority spot (550 > 500)
	fmt.Println("\n=== Повторный поиск 'go' / Re-querying 'go' ===")
	for i, res := range system.Search("go") {
		fmt.Printf("%d. %s\n", i+1, res)
	}
}
