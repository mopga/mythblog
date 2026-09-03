# Oddity Archive API for Hermes

Oddity Archive is an external HTTP content service. Hermes is only a client; LLM processing is not part of the backend runtime.

## Authentication

Use a dedicated `HERMES_API_KEY` bearer token:

```http
Authorization: Bearer $HERMES_API_KEY
Content-Type: application/json
```

The admin key has broader rights and should not be used by routine agent workflows.

## Base URLs

Development: `http://127.0.0.1:8890`
Production: configured HTTPS base URL.

Public URL for an article: `{base}/articles/{slug}`.

## Agent workflow

1. Research the user's topic; do not merely paraphrase one supplied URL.
2. Gather useful sources.
3. Check existing categories and content:
   - `GET /api/v1/categories`
   - `GET /api/v1/articles?slug=<slug>`
   - `GET /api/v1/articles?title=<title>`
4. Create missing categories if needed:
   - `POST /api/v1/categories`
5. If article exists: `PUT /api/v1/articles/{id}`. Otherwise: `POST /api/v1/articles`.
6. Upload useful media using the returned existing `article.id`:
   - `POST /api/v1/media`
7. Return the public article URL to the user.

## Categories

```http
POST /api/v1/categories
```

```json
{
  "slug": "ufo-uap",
  "name": "UFO/UAP",
  "description": "Unidentified aerial phenomena",
  "parent_id": null
}
```

## Media

```http
POST /api/v1/media
Content-Type: multipart/form-data
```

Fields:

- `article_id` — existing article ID;
- `file` — image file;
- `caption`, `alt_text`, `source_url`, `license`;
- `is_cover` — `true`/`false`;
- `sort_order` — integer.

The backend validates size and MIME, decodes and resizes the image, stores it through object storage and returns metadata plus a public URL.

## Article create/update

```http
POST /api/v1/articles
```

```json
{
  "slug": "phoenix-lights",
  "title": "Phoenix Lights",
  "subtitle": "Arizona, March 1997",
  "summary": "A mass sighting reported across Arizona.",
  "body_markdown": "## What is documented\n...",
  "type": "case",
  "status": "published",
  "credibility": "disputed",
  "event_date_text": "13 March 1997",
  "location_text": "Arizona, United States",
  "categories": ["ufo-uap"],
  "sources": [
    {
      "url": "https://example.org/report",
      "title": "Contemporary report",
      "publisher": "Example Archive",
      "published_at": "1997-03-14",
      "source_type": "report"
    }
  ],
  "relations": [
    {"related_article_id": 12, "relation": "related"}
  ]
}
```

Create/update is transactional. Categories, sources and relations are replaced with the supplied arrays. Relations to missing article IDs fail validation instead of creating empty placeholder articles.

## Errors

Errors are JSON and machine-readable:

```json
{
  "error": {
    "code": "validation_error",
    "message": "validation failed",
    "fields": {"slug": "must be a lowercase kebab-case slug"}
  }
}
```

Codes:

- `invalid_json`;
- `unauthorized`;
- `forbidden`;
- `not_found`;
- `conflict`;
- `validation_error`;
- `invalid_id`;
- `storage_error`;
- `internal_error`.

## Редакционная политика

Oddity Archive — архив легенд и странностей, а не сайт разоблачений.

- Сначала полно и атмосферно изложи саму историю: участники, место, время, последовательность событий и характерные детали.
- Собери разные версии и поздние пересказы, сохраняя их собственный голос.
- Источники нужны для происхождения и развития сюжета, а не для построения обвинительного перекрёстного фактчека.
- Не организуй статью вокруг вопроса «правда или ложь» и не повторяй после каждого эпизода, что доказательств недостаточно.
- Существенные противоречия или установленное художественное происхождение упоминай кратко, нейтрально и без торжества разоблачителя.
- Не выдавай легенду за установленный факт: используй `credibility` (`legend`, `disputed`, `confirmed`, `unknown`) и ясные формулировки «по легенде», «рассказ утверждает», «согласно этой версии».
- В финале предпочтительнее показать, почему история запомнилась и как менялась, а не вынести ей судебный приговор.
- Сохраняй URL, название, издателя и тип каждого источника.
- Не добавляй в `body_markdown` разделы `## Sources` или `## Источники`: сайт автоматически выводит единый блок «Источники» из структурированного массива `sources`.
- Загружай только уместные изображения; указывай `alt_text`, источник и лицензию, если они известны.
