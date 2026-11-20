package discovery

import "strings"

// engineImages maps a part of a container image name to a product name.
// Order matters: the first match wins, so more specific names come first.
var engineImages = []struct{ match, engine string }{
	{"postgres", "PostgreSQL"},
	{"mariadb", "MariaDB"},
	{"mysql", "MySQL"},
	{"mongo", "MongoDB"},
	{"redis", "Redis"},
	{"valkey", "Valkey"},
	{"elasticsearch", "Elasticsearch"},
	{"opensearch", "OpenSearch"},
	{"cassandra", "Cassandra"},
	{"mssql", "SQL Server"},
	{"minio", "MinIO"},
	{"rabbitmq", "RabbitMQ"},
	{"kafka", "Kafka"},
}

// detectEngine returns the data store product behind an image, or "" if none is recognised.
func detectEngine(images []string) string {
	for _, image := range images {
		name := imageName(image)
		for _, e := range engineImages {
			if strings.Contains(name, e.match) {
				return e.engine
			}
		}
	}
	return ""
}

// imageName strips registry, tag and digest: "docker.io/library/postgres:16" becomes "postgres".
func imageName(image string) string {
	if i := strings.IndexByte(image, '@'); i >= 0 {
		image = image[:i]
	}
	if i := strings.LastIndexByte(image, '/'); i >= 0 {
		image = image[i+1:]
	}
	if i := strings.IndexByte(image, ':'); i >= 0 {
		image = image[:i]
	}
	return strings.ToLower(image)
}
