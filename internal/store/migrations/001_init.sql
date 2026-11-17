CREATE TABLE settings (
  k VARCHAR(64) PRIMARY KEY,
  v TEXT NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE teams (
  id    VARCHAR(64)  PRIMARY KEY,
  name  VARCHAR(255) NOT NULL,
  color VARCHAR(16)  NOT NULL DEFAULT '',
  sort  INT          NOT NULL DEFAULT 0
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE people (
  id             VARCHAR(64)  PRIMARY KEY,
  name           VARCHAR(255) NOT NULL,
  short_name     VARCHAR(64)  NOT NULL DEFAULT '',
  team_id        VARCHAR(64)  NOT NULL,
  weekly_minutes INT          NOT NULL DEFAULT 0,
  tags           VARCHAR(255) NOT NULL DEFAULT '',
  fixed_json     TEXT         NOT NULL,
  sort           INT          NOT NULL DEFAULT 0,
  CONSTRAINT fk_people_team FOREIGN KEY (team_id) REFERENCES teams(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE activities (
  id    VARCHAR(64)  PRIMARY KEY,
  kind  VARCHAR(16)  NOT NULL,
  code  VARCHAR(8)   NOT NULL,
  label VARCHAR(255) NOT NULL,
  sort  INT          NOT NULL DEFAULT 0
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE rules (
  id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  type        VARCHAR(64)  NOT NULL,
  label       VARCHAR(255) NOT NULL,
  severity    VARCHAR(8)   NOT NULL DEFAULT 'hard',
  enabled     BOOL         NOT NULL DEFAULT TRUE,
  params_json TEXT         NOT NULL,
  sort        INT          NOT NULL DEFAULT 0
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE deviations (
  id        BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  rule_id   BIGINT UNSIGNED NOT NULL,
  person_id VARCHAR(64)  NOT NULL DEFAULT '',
  team_id   VARCHAR(64)  NOT NULL DEFAULT '',
  week      INT          NOT NULL DEFAULT 0,
  day       VARCHAR(3)   NOT NULL DEFAULT '',
  reason    TEXT         NOT NULL,
  CONSTRAINT fk_dev_rule FOREIGN KEY (rule_id) REFERENCES rules(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE plans (
  id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  name       VARCHAR(255) NOT NULL,
  created_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE plan_cells (
  plan_id   BIGINT UNSIGNED  NOT NULL,
  person_id VARCHAR(64)      NOT NULL,
  week      TINYINT UNSIGNED NOT NULL,
  weekday   TINYINT UNSIGNED NOT NULL,
  start_min SMALLINT         NOT NULL,
  end_min   SMALLINT         NOT NULL,
  acts      VARCHAR(255)     NOT NULL DEFAULT '',
  PRIMARY KEY (plan_id, person_id, week, weekday),
  CONSTRAINT fk_cells_plan FOREIGN KEY (plan_id) REFERENCES plans(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
