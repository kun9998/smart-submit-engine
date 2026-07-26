-- ============================================================
-- tj 提交插件独立数据库（与 install.sql 主站库分离）
-- 安装向导会自动创建数据库并执行本脚本
-- ============================================================

SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS = 0;

-- ----------------------------
-- 管理员账号
-- ----------------------------
DROP TABLE IF EXISTS `tj_admin_user`;
CREATE TABLE `tj_admin_user` (
  `id` int NOT NULL AUTO_INCREMENT,
  `username` varchar(64) NOT NULL COMMENT '登录用户名',
  `password_hash` varchar(255) NOT NULL COMMENT 'bcrypt 密码哈希',
  `showdoc_url` varchar(512) DEFAULT NULL COMMENT 'Showdoc 推送地址',
  `totp_secret` varchar(128) DEFAULT NULL COMMENT 'TOTP 密钥',
  `totp_enabled` tinyint(1) NOT NULL DEFAULT 0 COMMENT '是否启用 TOTP',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_username` (`username`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='插件管理后台用户';

-- ----------------------------
-- 平台提交规则
-- ----------------------------
DROP TABLE IF EXISTS `tj_submit_platform`;
CREATE TABLE `tj_submit_platform` (
  `id` int NOT NULL AUTO_INCREMENT,
  `platform_type` varchar(64) NOT NULL COMMENT '平台类型，对应主库 love_learn_huoyuan.pt',
  `display_name` varchar(128) NOT NULL DEFAULT '',
  `enabled` tinyint(1) NOT NULL DEFAULT 1,
  `rule_config` json NOT NULL,
  `version` int NOT NULL DEFAULT 1,
  `remark` varchar(512) NOT NULL DEFAULT '',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_platform_type` (`platform_type`),
  KEY `idx_enabled` (`enabled`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='平台提交规则';

-- ----------------------------
-- 系统元数据（运行时配置等）
-- ----------------------------
DROP TABLE IF EXISTS `tj_system_meta`;
CREATE TABLE `tj_system_meta` (
  `meta_key` varchar(64) NOT NULL,
  `meta_value` text NOT NULL,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`meta_key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='插件系统元数据';

-- ----------------------------
-- 货源运行时配置（hid 映射主库 love_learn_huoyuan.hid）
-- ----------------------------
DROP TABLE IF EXISTS `tj_huoyuan_runtime`;
CREATE TABLE `tj_huoyuan_runtime` (
  `hid` int NOT NULL COMMENT '货源ID',
  `config_json` json NOT NULL COMMENT '覆盖项 JSON',
  `remark` varchar(512) NOT NULL DEFAULT '',
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`hid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='货源运行时配置';

SET FOREIGN_KEY_CHECKS = 1;
