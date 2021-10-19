package database

import logrus "github.com/sirupsen/logrus"

func DaoFactory(daoType string) Dao {
	switch daoType {
	case "psql":
		return NewPSQLDao()

	default:
		logrus.Errorf("There is no current support for the daotype %s. Please select a different supported daotype", daoType)
		return nil
	}
}
