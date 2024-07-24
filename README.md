# matchmaking

create database matchmaking
with owner postgres;

create table public.users
(
id bigserial
constraint users_pk
primary key
);

alter table public.users
owner to postgres;

create table public.matching_games
(
user_id bigint  not null
constraint matching_games_users_id_fk
references public.users,
level   integer not null,
table_g integer not null,
constraint matching_games_pk
unique (table_g, user_id, level)
)
partition by LIST (table_g);

alter table public.matching_games
owner to postgres;

create table public.matchmaking_table_44
partition of public.matching_games
(
constraint matching_games_users_id_fk
foreign key (user_id) references public.users
)
FOR VALUES IN (44);

alter table public.matchmaking_table_44
owner to postgres;

create table public.matchmaking_table_1
partition of public.matching_games
(
constraint matching_games_users_id_fk
foreign key (user_id) references public.users
)
FOR VALUES IN (1);

alter table public.matchmaking_table_1
owner to postgres;

