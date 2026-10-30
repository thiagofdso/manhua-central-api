INSERT INTO canonical_manhuas (
    id, slug, title, alternative_titles, synopsis, cover_url,
    primary_domain, url_template, last_chapter, update_day,
    is_hiatus, bayesian_rating, total_votes, total_views
) VALUES
(
    1792536414041,
    'pick-me-up-infinite-gacha',
    'Pick Me Up Infinite Gacha',
    'Pick Me Up, Infinite Gacha | Pick Me Up!',
    'In the mobile gacha game ''Pick Me Up!'', notorious master player Loki is mysteriously transported into the game world as a 1-star hero. To survive and return to reality, he must lead his comrades to conquer the 100th floor of the Town.',
    'https://w20.pickmeupgacha.com/wp-content/uploads/2023/12/Pick-Me-Up-Infinite-Gacha.jpg',
    'pickmeupgacha.com',
    'https://w19.pickmeupgacha.com/pick-me-up-infinite-gacha-chapter-{chapter}/',
    224,
    4,
    false,
    4.90,
    120,
    45000
),
(
    1792536751018,
    'magic-emperor-online',
    'Magic Emperor',
    'Demonic Emperor | The Devil Butler | Mo Huang Da Guan Jia',
    'Zhuo Yifan was the Demon Emperor. After discovering the ancient secret book of the Nine Serenities, he was betrayed and killed by his closest disciple. He is reborn in the body of a loyal servant, Zhuo Fan, of the decaying Luo Clan.',
    'https://magicemperor.online/wp-content/uploads/2025/03/Magic-Emperor-Wallpaper.jpg',
    'magicemperor.online',
    'https://magicemperor.online/manga/magic-emperor-chapter-{chapter}/',
    925,
    6,
    false,
    4.85,
    95,
    38000
),
(
    1792537028351,
    'leveling-up-with-the-gods-online',
    'Leveling Up With The Gods',
    'Leveling with the Gods | Sin-gwa Hamgge Rebeul-eob',
    'Kim YuWon was one of the strongest inner circle climbers of the Tower. After facing impossible odds against the Outer Gods, his companion Chronos rewound time. Reborn in the past, YuWon climbs the Tower again, determined to save the world.',
    'https://w17.levelingwithgods.com/wp-content/uploads/2023/08/Manga-Covers-1-1.png',
    'levelingwithgods.com',
    'https://w14.levelingwithgods.com/manga/leveling-with-the-gods-chapter-{chapter}/',
    170,
    NULL,
    true,
    4.75,
    70,
    29000
)
ON CONFLICT (slug) DO UPDATE SET
    cover_url = EXCLUDED.cover_url,
    url_template = EXCLUDED.url_template,
    last_chapter = EXCLUDED.last_chapter,
    update_day = EXCLUDED.update_day,
    is_hiatus = EXCLUDED.is_hiatus;

INSERT INTO catalog_sources (canonical_id, domain, source_url, original_cover_url, next_attempt_at)
VALUES
(1792536414041, 'pickmeupgacha.com', 'https://w19.pickmeupgacha.com/pick-me-up-infinite-gacha-chapter-1/', 'https://w20.pickmeupgacha.com/wp-content/uploads/2023/12/Pick-Me-Up-Infinite-Gacha.jpg', NOW()),
(1792536751018, 'magicemperor.online', 'https://magicemperor.online/manga/magic-emperor-chapter-1/', 'https://magicemperor.online/wp-content/uploads/2025/03/Magic-Emperor-Wallpaper.jpg', NOW()),
(1792537028351, 'levelingwithgods.com', 'https://w14.levelingwithgods.com/manga/leveling-with-the-gods-chapter-1/', 'https://w17.levelingwithgods.com/wp-content/uploads/2023/08/Manga-Covers-1-1.png', NOW())
ON CONFLICT (domain, source_url) DO NOTHING;
