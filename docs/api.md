# API первой версии

JSON, один origin с интерфейсом. Изменяющие запросы отправляют заголовок
`X-Campus-Request: 1`. Ошибка: `{ "error": "Сообщение для пользователя" }`.
Сессия в HttpOnly SameSite=Strict cookie; сервер определяет роль.

- `GET /api/session` → `{user: PublicUser|null}`.
- `POST /api/login` `{username,password}` → `{user}`.
- `POST /api/register` `{username,name,password,invite_code}` → `{user}`.
- `POST /api/logout` → `{ok:true}`.
- `GET /api/state` → `{user,users,groups,topics,problems,works,submissions,languages,server_time,judging_mode:"automatic"|"manual",demo:boolean}`.
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
Для ручной проверки преподаватель может выставить verdict, кроме pending и judge_error.
Дополнительные вердикты: output_limit и judge_error; последний означает сбой
исполнителя и не считается неудачной попыткой.
Статус pending означает ожидание проверки. Автоматическая посылка содержит
`judging:{status,passed_tests,total_tests,failed_test?,time_ms,compile_output?,error?}`.
Status: queued, running, done, error. `time_ms` — максимальное время одного теста,
без подготовки контейнера. Скрытые входы и ожидаемые ответы не возвращаются.
Ответ POST приходит после сохранения посылки в очереди, а не после исполнения;
GET /api/state показывает переходы и итог. Ручной результат снимает judging и
не может быть перезаписан поздним worker.

Языки — список `{id,name,automatic,template}`; принимается также `other`.
`automatic` учитывает текущий режим сервера и наличие исполнителя языка;
`template` — полная программа для stdin/stdout. Источник ограничен 128 КиБ.
Для автоматического языка нужен хотя бы один пример или скрытый тест. Очередь
ограничена 64 активными посылками и тремя на ученика; превышение даёт HTTP429.
В остальных языках посылка сохраняется для ручной проверки без judging. Публикация запрещена при пустом списке групп
или задач. Сроки проверяются сервером. До начала контеста ученик видит карточку,
но не условия и посылки. После окончания доступны условия, история и таблица,
новые посылки отклоняются.
