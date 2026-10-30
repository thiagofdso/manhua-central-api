ALTER TABLE user_ratings DROP CONSTRAINT IF EXISTS user_ratings_score_check;
ALTER TABLE user_ratings ADD CONSTRAINT user_ratings_score_check CHECK (score >= 1 AND score <= 10);
