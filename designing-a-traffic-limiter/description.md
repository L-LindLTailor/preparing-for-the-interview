# Проектирование ограничителя трафика / designing a traffic limiter.

## Реализовано несколько алгоритмов:
* [счетчик скользящих интервалов (sliding window counter)](#счетчик-скользящих-интервалов-sliding-window-counter). [Ссылка на раздел / Link to the section"](sliding-window-counter/);
* [алгоритм дырявого ведра (leaking bucket)](#алгоритм-дырявого-ведра-leaking-bucket). [Ссылка на раздел / Link to the section"](leaking-bucket/);
* [счетчик фиксированных интервалов (fixed window counter)](#счетчик-фиксированных-интервалов-fixed-window-counter). [Ссылка на раздел / Link to the section"](fixed-window-counter/);
* [алгоритм маркерной корзины (token bucket)](#алгоритм-маркерной-корзины-token-bucket). [Ссылка на раздел / Link to the section"](token-bucket/);
* [журнал скользящих интервалов (sliding window log)](#журнал-скользящих-интервалов-sliding-window-log). [Ссылка на раздел / Link to the section"](sliding-window-log/).

## Счетчик скользящих интервалов (sliding window counter)

## Алгоритм дырявого ведра (leaking bucket)

## Счетчик скользящих интервалов (sliding window counter)

## Алгоритм маркерной корзины (token bucket)

### Алгоритм Token Bucket (Маркерная корзина) с ленивым пополнением (Lazy Refill) разработан специально для высоконагруженных систем и защиты от DDoS-атак. Его ключевое отличие от классической реализации на каналах и таймерах — полное отсутствие фоновых потоков (горутин).

### Фоновые таймеры создают огромную нагрузку на планировщик и память при наплыве миллионов запросов. Вместо этого данный алгоритм использует чистую математику времени в момент прихода запроса:

* Идентификация: При поступлении HTTP-запроса система извлекает уникальный ключ клиента (API-key, User-ID или IP).
* Ленивое пополнение (Lazy Refill): Вместо ежесекундного добавления токенов, корзина «вспоминает», когда к ней обращались в последний раз. Разница между текущим временем и временем прошлого запроса (elapsed) умножается на скорость генерации токенов (refillRate). Полученное число виртуально добавляется к балансу.
* Защита от всплесков (Burst Limitation): Баланс токенов жестко ограничивается максимальной емкостью (capacity). Это позволяет пользователю совершить кратковременную серию запросов (всплеск) после периода простоя, но предотвращает бесконечное накопление лимитов.
* Мгновенное решение (O(1) по памяти и времени): Если после пополнения баланс содержит хотя бы \(1\) токен, он уменьшается на \(1\), и запрос немедленно пропускается. Если токенов нет, функция мгновенно возвращает false, а сервер отвечает кодом 429 Too Many Requests. Потоки сервера не блокируются, память под ожидание не выделяется, что делает систему неуязвимой для OOM (Out of Memory) при DDoS.

### The Token Bucket algorithm with Lazy Refill is explicitly designed for high-throughput systems and DDoS mitigation. Its core advantage over traditional channel- or timer-based implementations is the complete elimination of background threads (goroutines).

### Background tickers degrade performance and consume unpredictable amounts of memory when processing millions of concurrent requests. Instead, this architecture relies on lazy evaluation based on timestamps:

* Identification: When an HTTP request arrives, the system extracts a unique identifier (API-key, User-ID, or IP)
* Lazy Refill: Rather than continuously pushing tokens into a channel via a background loop, the bucket updates its state reactively. It calculates the duration since the last check (elapsed) and multiplies it by the refillRate. This mathematically yields the number of tokens generated during the idle period.
* Burst Boundary Control: The token balance is strictly capped at the maximum capacity. This allows legitimate clients to execute rapid back-to-back requests (bursts) after inactivity while ensuring that tokens cannot accumulate infinitely.
* Immediate Boundary Response (\(O(1)\) Complexity): If the updated balance holds at least \(1.0\) token, the algorithm decrements it by \(1\) and immediately grants access. If insufficient, it abruptly returns false, triggers an HTTP 429 Too Many Requests error, and frees up execution threads instantly. No goroutines are left parked or waiting, rendering the application highly resilient against Out-of-Memory (OOM) failures under heavy DDoS conditions.

## Журнал скользящих интервалов (sliding window log)
