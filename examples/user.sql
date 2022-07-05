CREATE TABLE `user` (
  `id` bigint NOT NULL AUTO_INCREMENT COMMENT 'primary key',
  `tenant_id` bigint NOT NULL DEFAULT 0 COMMENT 'tenant id',
  `email` varchar(128) NOT NULL COMMENT 'email',
  `name` varchar(64) NOT NULL COMMENT 'display name',
  `status` tinyint NOT NULL DEFAULT 1 COMMENT 'status',
  `avatar_url` varchar(255) DEFAULT NULL COMMENT 'avatar URL',
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_email` (`email`),
  UNIQUE KEY `uk_tenant_email` (`tenant_id`, `email`),
  KEY `idx_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
