-- Keep existing rows and foreign keys while transferring ownership to Seller Service.
ALTER TABLE public.sellers SET SCHEMA seller;
ALTER TABLE public.seller_members SET SCHEMA seller;
