-- +goose Up
-- +goose StatementBegin
ALTER TABLE users ADD COLUMN n INTEGER NOT NULL DEFAULT 1;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
SELECT 'down SQL query';
-- +goose StatementEnd
