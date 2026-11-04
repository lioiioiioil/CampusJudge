# API первой версии

JSON, один origin с интерфейсом. Изменяющие запросы отправляют заголовок
`X-Campus-Request: 1`. Ошибка: `{ "error": "Сообщение для пользователя" }`.
Сессия в HttpOnly SameSite=Strict cookie; сервер определяет роль.

- `GET /api/session` → `{user: PublicUser|null}`.
- `POST /api/login` `{username,password}` → `{user}`.
- `POST /api/register` `{username,name,password,invite_code}` → `{user}`.
- `POST /api/logout` → `{ok:true}`.
- `GET /api/state` → `{user,users,groups,topics,problems,works,submissions,languages,server_time,judging_mode:"manual",demo:boolean}`.
  Темы и ссылки на занятия доступны зарегистрированным ученикам сразу.
  Ученику приходят только разрешённые работы, их задачи без скрытых тестов,
  собственные посылки и собственный профиль. Преподавателю — все данные класса.
- `POST /api/groups` `{name}` → созданная группа с invite_code.
- `POST /api/topics` `{title,description,lesson_url?:https URL}` → тема. Ссылка открывается во внешнем видеосервисе.
- `POST /api/problems` `{title,statement,input_format,output_format,time_limit_ms,memory_limit_mb,samples:[{input,output}],tests:[{input,output}]}` → задача.
- `POST /api/works` `{title,description,kind:"homework"|"contest",topic_id,group_ids:[],problem_ids:[],status:"draft"|"open"|"closed",starts_at?:RFC3339,ends_at?:RFC3339}` → работа.
- `PATCH /api/works/{id}` `{status:"draft"|"open"|"closed"}` → работа.
- `POST /api/submissions` `{work_id,problem_id,language,source}` → посылка.
- `PATCH /api/submissions/{id}` `{verdict,feedback}` → проверенная посылка.
- `GET /api/works/{id}/standings` → `{rows:[{user_id,name,solved,penalty,problems:{problem_id:{solved:boolean,attempts:number,minutes:number}}}]}`.

Сущности определены в `internal/model/model.go`. Verdict: `pending`, `accepted`,
`wrong_answer`, `compilation_error`, `runtime_error`, `time_limit`, `memory_limit`.
Для ручной проверки преподаватель может выставить verdict, кроме pending.
Статус pending означает ожидание проверки; выполнение исходников пока отсутствует.

Языки — список `{id,name}`; принимается также `other`. Приём текста на языке
не равнозначен автоматической проверке. Публикация запрещена при пустом списке групп
или задач. Сроки проверяются сервером. До начала контеста ученик видит карточку,
но не условия и посылки. После окончания доступны условия, история и таблица,
новые посылки отклоняются.
