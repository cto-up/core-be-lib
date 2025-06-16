ALTER TABLE public.core_users 
ADD COLUMN IF NOT EXISTS roles VARCHAR(20)[];

CREATE INDEX IF NOT EXISTS idx_core_users_roles 
ON public.core_users USING GIN (roles);
