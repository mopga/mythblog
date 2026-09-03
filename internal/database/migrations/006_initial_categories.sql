INSERT OR IGNORE INTO categories(slug, name, description, parent_id, created_at) VALUES
('paranormalnoe', 'Паранормальное', 'Истории, духи и необъяснимые явления', NULL, strftime('%Y-%m-%dT%H:%M:%fZ','now')),
('sushchestva', 'Существа', 'Криптиды, демоны и нежить', NULL, strftime('%Y-%m-%dT%H:%M:%fZ','now')),
('mify-i-legendy', 'Мифы и легенды', 'Фольклор разных народов мира', NULL, strftime('%Y-%m-%dT%H:%M:%fZ','now')),
('zagovory', 'Теории заговора', 'Версии, слухи и поздние пересказы', NULL, strftime('%Y-%m-%dT%H:%M:%fZ','now')),
('sekretnye-operatsii', 'Секретные операции', 'Программы, эксперименты и документы', NULL, strftime('%Y-%m-%dT%H:%M:%fZ','now')),
('ufo-uap', 'UFO / UAP', 'Наблюдения и неопознанные объекты', NULL, strftime('%Y-%m-%dT%H:%M:%fZ','now'));
