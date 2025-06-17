-- Remove the old core_roles column
ALTER TABLE public.core_users ADD COLUMN core_roles uuid[] NULL;

