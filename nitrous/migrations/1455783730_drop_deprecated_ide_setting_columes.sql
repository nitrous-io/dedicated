-- drop_deprecated_ide_setting_columes
-- Version: 1455783730

ALTER TABLE user_settings DROP COLUMN auto_close_brackets;
ALTER TABLE user_settings DROP COLUMN highlight_line;
ALTER TABLE user_settings DROP COLUMN keybindings;
ALTER TABLE user_settings DROP COLUMN match_brackets;
ALTER TABLE user_settings DROP COLUMN ruler_column;
ALTER TABLE user_settings DROP COLUMN show_line_numbers;
ALTER TABLE user_settings DROP COLUMN show_trailing_space;
ALTER TABLE user_settings DROP COLUMN soft_tabs;
ALTER TABLE user_settings DROP COLUMN tab_size;
ALTER TABLE user_settings DROP COLUMN theme;
ALTER TABLE user_settings DROP COLUMN wrap_lines;
ALTER TABLE user_settings DROP COLUMN remove_trailing_whitespace_on_save;
ALTER TABLE user_settings DROP COLUMN migrated;
