-- The core migration 0008 created these tables in public. Keep that applied
-- migration unchanged; Chat Service takes ownership from this version onward.
ALTER TABLE public.chat_conversations SET SCHEMA chat;
ALTER TABLE public.chat_messages SET SCHEMA chat;
ALTER TABLE public.chat_reads SET SCHEMA chat;
