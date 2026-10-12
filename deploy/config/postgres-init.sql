-- database-per-service. Прямой доступ к чужой базе запрещён —
-- межсервисное общение только через gRPC или Kafka.
CREATE DATABASE identity;
CREATE DATABASE catalog;
CREATE DATABASE cart;
CREATE DATABASE payment;
CREATE DATABASE "order";
CREATE DATABASE example;
