-- Core migrations 0001-0009 created and used these tables in public.
-- Identity Service owns their schema from this migration onward.
ALTER TABLE public.users SET SCHEMA identity;
ALTER TABLE public.roles SET SCHEMA identity;
ALTER TABLE public.user_roles SET SCHEMA identity;
ALTER TABLE public.shipping_addresses SET SCHEMA identity;
