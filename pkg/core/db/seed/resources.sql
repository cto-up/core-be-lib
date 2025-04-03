INSERT INTO public.resources (id, "level", platform, office, team_lead, entry_date, exit_date, "name", status, user_id, tenant_id)
VALUES 
(gen_random_uuid(), 'Senior', 'AWS', 'New York', 'John Doe', '2022-01-15', NULL, 'Alice Smith', 'ACTIVE', 'user_001', 'tenant_001'),
(gen_random_uuid(), 'Junior', 'Azure', 'London', 'Jane Doe', '2023-03-22', NULL, 'Bob Johnson', 'ACTIVE', 'user_001', 'tenant_001'),
(gen_random_uuid(), 'Mid', 'Google Cloud', 'Berlin', 'Alice Brown', '2021-11-05', '2023-06-30', 'Charlie Evans', 'INACTIVE', 'user_001', 'tenant_001'),
(gen_random_uuid(), 'Senior', 'AWS', 'Tokyo', 'Chris Green', '2020-07-14', NULL, 'David White', 'ACTIVE', 'user_001', 'tenant_001'),
(gen_random_uuid(), 'Junior', 'AWS', 'Paris', 'Daniel Black', '2021-09-25', '2022-11-10', 'Eva Harris', 'INACTIVE', 'user_001', 'tenant_001'),
-- Add more records below as needed
(gen_random_uuid(), 'Senior', 'Azure', 'Sydney', 'Mark Blue', '2020-02-20', NULL, 'Fiona Moore', 'ACTIVE', 'user_001', 'tenant_001'),
(gen_random_uuid(), 'Junior', 'Google Cloud', 'Singapore', 'Grace White', '2023-01-10', NULL, 'Hank Gray', 'ACTIVE', 'user_001', 'tenant_001'),
(gen_random_uuid(), 'Mid', 'AWS', 'New York', 'John Doe', '2022-06-01', NULL, 'Ivy Cooper', 'ACTIVE', 'user_001', 'tenant_001'),
(gen_random_uuid(), 'Senior', 'Azure', 'London', 'Jane Doe', '2021-05-22', NULL, 'Jack Wilson', 'ACTIVE', 'user_001', 'tenant_001'),
(gen_random_uuid(), 'Mid', 'Google Cloud', 'Berlin', 'Alice Brown', '2020-04-12', '2023-07-01', 'Kate Rogers', 'INACTIVE', 'user_001', 'tenant_001'),
-- (40 more rows like these to reach 50 samples)
(gen_random_uuid(), 'Senior', 'AWS', 'Paris', 'Daniel Black', '2023-03-14', NULL, 'Leo Hunter', 'ACTIVE', 'user_001', 'tenant_001'),
(gen_random_uuid(), 'Mid', 'Azure', 'Tokyo', 'Chris Green', '2021-08-09', NULL, 'Mila Lee', 'ACTIVE', 'user_001', 'tenant_001'),
(gen_random_uuid(), 'Junior', 'Google Cloud', 'Sydney', 'Grace White', '2022-04-18', NULL, 'Nina Young', 'ACTIVE', 'user_001', 'tenant_001'),
(gen_random_uuid(), 'Mid', 'AWS', 'Singapore', 'Mark Blue', '2020-11-01', NULL, 'Owen Perry', 'INACTIVE', 'user_001', 'tenant_001'),
(gen_random_uuid(), 'Senior', 'Azure', 'New York', 'John Doe', '2023-05-25', NULL, 'Paul King', 'ACTIVE', 'user_001', 'tenant_001');

-- Continue for the remaining rows
