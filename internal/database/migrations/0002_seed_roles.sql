INSERT INTO roles (name) VALUES ('customer') ON CONFLICT (name) DO NOTHING;
INSERT INTO roles (name) VALUES ('seller_admin') ON CONFLICT (name) DO NOTHING;
INSERT INTO roles (name) VALUES ('admin') ON CONFLICT (name) DO NOTHING;
