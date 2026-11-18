-- Retire broadcast messages and their delivery queue; email verification is unchanged.
DROP TABLE IF EXISTS mail_deliveries;
DROP TABLE IF EXISTS communications;
