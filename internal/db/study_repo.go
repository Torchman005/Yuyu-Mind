package db

import "database/sql"

// StudyRepo 学习数据仓储的 DB 包装
type StudyRepo struct {
	db *sql.DB
}

// NewStudyRepo 创建学习数据仓储
func NewStudyRepo(db *sql.DB) *StudyRepo {
	return &StudyRepo{db: db}
}

// SQL 返回底层 SQL 连接供 study.Repository 使用
func (r *StudyRepo) SQL() *sql.DB {
	return r.db
}
