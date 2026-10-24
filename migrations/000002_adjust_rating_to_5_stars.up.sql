-- Adjust score constraint on user_ratings to 1-5 stars (ADR-0013 / RN-07)
ALTER TABLE user_ratings DROP CONSTRAINT IF EXISTS user_ratings_score_check;
ALTER TABLE user_ratings DROP CONSTRAINT IF EXISTS check_score_range;

-- Cap any existing score > 5 to 5 before enforcing new constraint
UPDATE user_ratings SET score = 5 WHERE score > 5;

ALTER TABLE user_ratings ADD CONSTRAINT user_ratings_score_check CHECK (score >= 1 AND score <= 5);

-- Adjust existing canonical_manhuas bayesian_rating to 5-star scale if > 5
UPDATE canonical_manhuas SET bayesian_rating = ROUND((bayesian_rating / 2.0)::numeric, 2) WHERE bayesian_rating > 5.0;
