# Сквозной пример: Обработка заказа с конвертацией валюты

В этом документе описан сквозной пример настройки ESB для решения следующей бизнес-задачи:

1. **Приложение 1 ("Магазин")** отправляет сообщение о новом заказе клиента. Сумма заказа указана в долларах (USD).
2. **Трансформация** получает сообщение о заказе, сама запрашивает актуальный курс USD к EUR от внешнего сервиса, применяет его к сумме заказа и обогащает сообщение.
3. **Ветвление логики:** В зависимости от суммы заказа в EUR, сообщение направляется в разные системы:
    * Если сумма > 1000 EUR, заказ отправляется в **Приложение 2 ("Отдел логистики VIP")**.
    * Если сумма <= 1000 EUR, заказ отправляется в **Приложение 3 ("Стандартная логистика")**.

## Шаг 1: Настройка приложений и каналов

Сначала необходимо создать три приложения и необходимые для них каналы в админ-панели.

1. **Приложение 1: `StoreApp`**
    * **Канал 1:**
        * Название: `new_orders`
        * Направление: `outbound` (Магазин отправляет заказы *из* себя *в* шину)
        * Назначение (очередь): `q.store.new_orders`
        * **Режим Fan-out:** ✓ **Включен**. Это критически важно, так как мы хотим, чтобы сообщение о заказе было доступно нескольким маршрутам-фильтрам для реализации ветвления.

2. **Приложение 2: `VIPLogisticsApp`**
    * **Канал 1:**
        * Название: `incoming_vip`
        * Направление: `inbound` (Приложение получает заказы *из* шины)
        * Назначение (очередь): `q.logistics.vip_orders`
        * **Режим Fan-out:** ✗ Выключен.

3. **Приложение 3: `StandardLogisticsApp`**
    * **Канал 1:**
        * Название: `incoming_standard`
        * Направление: `inbound`
        * Назначение (очередь): `q.logistics.standard_orders`
        * **Режим Fan-out:** ✗ Выключен.

## Шаг 2: Трансформация и ветвление

Реализуем логику ветвления с помощью двух маршрутов-фильтров. Для получения актуального курса валют скрипты трансформации будут сами обращаться к внешнему API.

### Трансформация 1: `TransformToEUR_HighValue`

Этот скрипт обработает заказ, запросит актуальный курс USD к EUR, сконвертирует сумму и вернет сообщение, только если сумма превышает 1000 EUR.

* **Название:** `TransformToEUR_HighValue`
* **Движок:** `starlark`
* **Скрипт:**

```python
# Задаем пороговое значение
THRESHOLD = 1000.0

def get_exchange_rate():
    """
    Получает курс USD к EUR через внешний API.
    Использует API ExchangeRate-API.com
    Возвращает курс или 0.93 в случае ошибки/недоступности.
    """
    log.info("Fetching USD to EUR exchange rate...")
    response = http.get(url="https://api.exchangerate-api.com/v4/latest/USD")

    if response.status_code != 200:
        log.error("Failed to fetch currency rates. Status: " + str(response.status_code) + ". Body: " + response.body)
        return 0.93 # Курс по умолчанию в случае ошибки

    body = json.decode(response.body)
    rates = body.get("rates", {})
    eur_rate = rates.get("EUR")

    if not eur_rate:
        log.error("EUR rate not found in API response. Using default.")
        return 0.93

    log.info("Successfully fetched USD to EUR rate: " + str(eur_rate))
    return eur_rate


def transform(body, headers):
    """
    Трансформирует сумму заказа в EUR и фильтрует по порогу.
    """
    order_total_usd = body.get("total_usd", 0.0)

    # Получаем актуальный курс
    exchange_rate = get_exchange_rate()

    # Конвертируем сумму
    order_total_eur = order_total_usd * exchange_rate

    # Проверяем условие
    if order_total_eur > THRESHOLD:
        # Сумма большая, обогащаем и возвращаем сообщение
        body["total_eur"] = order_total_eur
        body["currency"] = "EUR"
        body["value_category"] = "high"
        log.info("High value order processed: " + str(order_total_eur) + " EUR")
        return {
            "body": body,
            "headers": headers
        }
    else:
        # Сумма маленькая, отфильтровываем сообщение, возвращая None
        log.info("Low value order filtered: " + str(order_total_eur) + " EUR")
        return None
```

### Трансформация 2: `TransformToEUR_LowValue`

Аналогичный скрипт, но с обратным условием.

* **Название:** `TransformToEUR_LowValue`
* **Движок:** `starlark`
* **Скрипт:**

```python
# Задаем пороговое значение
THRESHOLD = 1000.0

def get_exchange_rate():
    """
    Получает курс USD к EUR через внешний API.
    Использует API ExchangeRate-API.com
    Возвращает курс или 0.93 в случае ошибки/недоступности.
    """
    log.info("Fetching USD to EUR exchange rate...")
    response = http.get(url="https://api.exchangerate-api.com/v4/latest/USD")

    if response.status_code != 200:
        log.error("Failed to fetch currency rates. Status: " + str(response.status_code) + ". Body: " + response.body)
        return 0.93 # Курс по умолчанию в случае ошибки

    body = json.decode(response.body)
    rates = body.get("rates", {})
    eur_rate = rates.get("EUR")

    if not eur_rate:
        log.error("EUR rate not found in API response. Using default.")
        return 0.93

    log.info("Successfully fetched USD to EUR rate: " + str(eur_rate))
    return eur_rate


def transform(body, headers):
    """
    Трансформирует сумму заказа в EUR и фильтрует по порогу.
    """
    order_total_usd = body.get("total_usd", 0.0)
    exchange_rate = get_exchange_rate()
    order_total_eur = order_total_usd * exchange_rate

    # Проверяем обратное условие
    if order_total_eur <= THRESHOLD:
        body["total_eur"] = order_total_eur
        body["currency"] = "EUR"
        body["value_category"] = "low"
        log.info("Low value order processed: " + str(order_total_eur) + " EUR")
        return {
            "body": body,
            "headers": headers
        }
    else:
        log.info("High value order filtered: " + str(order_total_eur) + " EUR")
        return None
```

## Шаг 3: Создание маршрутов

Свяжем все вместе с помощью двух маршрутов.

1. **Маршрут 1: `Route_HighValueOrders`**
    * **Название:** `Route_HighValueOrders`
    * **Источник:** `Источник (внешний): StoreApp / new_orders` (наш канал с флагом Fan-out)
    * **Тип маршрута:** С трансформацией (Transform)
    * **Трансформация:** `TransformToEUR_HighValue`
    * **Назначение:** `VIPLogisticsApp -> incoming_vip`

2. **Маршрут 2: `Route_LowValueOrders`**
    * **Название:** `Route_LowValueOrders`
    * **Источник:** **Тот же самый** - `Источник (внешний): StoreApp / new_orders`
    * **Тип маршрута:** С трансформацией (Transform)
    * **Трансформация:** `TransformToEUR_LowValue`
    * **Назначение:** `StandardLogisticsApp -> incoming_standard`

## Шаг 4: Тестирование

Теперь, если `StoreApp` отправит в шину сообщение (например, через тестовую форму на странице канала):

**Сообщение 1 (Большой заказ):**

```json
{
  "order_id": "ORD-001",
  "customer": "John Doe",
  "total_usd": 5000.0
}
```

Это сообщение будет обработано маршрутом `Route_HighValueOrders`, трансформировано и отправлено в канал `incoming_vip` для `VIPLogisticsApp`. Второй маршрут его отфильтрует.

**Сообщение 2 (Маленький заказ):**

```json
{
  "order_id": "ORD-002",
  "customer": "Jane Smith",
  "total_usd": 500.0
}
```

Это сообщение будет обработано маршрутом `Route_LowValueOrders`, трансформировано и отправлено в канал `incoming_standard` для `StandardLogisticsApp`.

Таким образом, мы реализовали сложный сценарий с получением данных, трансформацией и условным ветвлением логики, используя только каналы и трансформации.
