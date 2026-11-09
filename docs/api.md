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
  При `?summary=1` поле `source` исключается из посылок.
- `POST /api/groups` `{name}` → созданная группа с invite_code.
- `POST /api/topics` `{title,description,lesson_url?:https URL}` → тема. Ссылка открывается во внешнем видеосервисе.
- `POST /api/problems` `{title,statement,input_format,output_format,time_limit_ms,memory_limit_mb,samples:[{input,output}],tests:[{input,output}]}` → задача.
- `POST /api/problems/{id}/test-revisions` `{samples,tests}` → новая неизменяемая версия тестов. Исходные тесты считаются версией 1.
- `POST /api/works` `{title,description,kind:"homework"|"contest",topic_id,group_ids:[],problem_ids:[],status:"draft"|"open"|"closed",starts_at?:RFC3339,ends_at?:RFC3339}` → работа.
- `PATCH /api/works/{id}` `{status:"draft"|"open"|"closed"}` → работа.
- `PATCH /api/works/{id}/problem-revisions` `{problem_id,revision}` → работа с версией для новых посылок. Уже отправленные посылки сохраняют свою версию.
- `PATCH /api/works/{id}/draft` `{title,description,topic_id,group_ids:[],problem_ids:[],starts_at?:RFC3339,ends_at?:RFC3339}` → обновлённый черновик.
  Метод доступен преподавателю. Сохраняет тип и статус работы. Отсутствующие сроки сбрасываются.
  Открытая или закрытая работа и черновик с посылками дают `409`.
- `POST /api/submissions` `{work_id,problem_id,language,source}` → посылка.
- `GET /api/submissions/{id}` → `{user_id,submission}` с исходником. Преподаватель
  видит любую посылку, ученик — свою посылку в доступной работе.
- `GET /api/submission-statuses?ids=id1,id2` → `{user_id,submissions}` без исходников.
  Принимает до 64 идентификаторов. Недоступные посылки исключаются из ответа.
- `PATCH /api/submissions/{id}` `{verdict,feedback}` → проверенная посылка.
- `POST /api/submissions/{id}/retry` → `202` и та же посылка в очереди после сбоя исполнителя. Действие преподавателя сохраняет исходник, время отправки и версию тестов.
- `POST /api/submissions/{id}/rejudge` `{revision}` → `202` и та же посылка в очереди на выбранную версию. Текущий вердикт действует до успешного завершения перепроверки. Смена результата сохраняет предыдущий вердикт в `past_judgments`.
- `GET /api/works/{id}/standings` → `{rows:[{user_id,name,solved,penalty,problems:{problem_id:{solved:boolean,attempts:number,minutes:number}}}]}`.

Сущности определены в `internal/model/model.go`. Verdict: `pending`, `accepted`,
`wrong_answer`, `compilation_error`, `runtime_error`, `time_limit`, `memory_limit`.
Для ручной проверки преподаватель может выставить verdict, кроме pending и judge_error.
Дополнительные вердикты: output_limit и judge_error; последний означает сбой
исполнителя и не считается неудачной попыткой.
Статус pending означает ожидание проверки. Автоматическая посылка содержит
`judging:{status,target_revision?,passed_tests,total_tests,failed_test?,time_ms,compile_output?,error?}`.
`test_revision` относится к действующему результату. Во время перепроверки
`judging.target_revision` указывает версию новой попытки.
Status: queued, running, done, error. `time_ms` — максимальное время одного теста,
без подготовки контейнера. Скрытые входы и ожидаемые ответы не возвращаются.
Ответ POST приходит после сохранения посылки в очереди, а не после исполнения;
Интерфейс получает переходы и итог через `/api/submission-statuses`.
Ручной результат снимает `judging` и имеет приоритет при завершении worker.

Языки — список `{id,name,automatic,template}`; принимается также `other`.
`automatic` учитывает текущий режим сервера и наличие исполнителя языка;
`template` — полная программа для stdin/stdout. Источник ограничен 128 КиБ.
Для автоматического языка нужен хотя бы один пример или скрытый тест. Очередь
ограничена 64 активными посылками и тремя на ученика; превышение даёт HTTP429.
В остальных языках посылка сохраняется для ручной проверки без judging. Публикация запрещена при пустом списке групп
или задач. Сроки проверяются сервером. До начала контеста ученик видит карточку,
но не условия и посылки. После окончания доступны условия, история и таблица,
новые посылки отклоняются.

## Пробный запуск

- `POST /api/trials` принимает `{actor_id,work_id,problem_id,language,source,sample_index,test_revision?}`
  для открытого примера. Вместо `sample_index` можно передать `input`, включая пустую строку.
  Номер примера начинается с нуля и относится к активной версии тестов в работе.
  Если переданный `test_revision` отличается от активной версии, сервер возвращает
  HTTP 409. Сервер сверяет `actor_id` с текущей сессией и проверяет доступ ученика
  к задаче в открытой работе.
- Успешный ответ содержит HTTP 202 и `{id,status:"running"}`.
  `GET /api/auxiliary-runs/{id}` возвращает текущее состояние владельцу запуска.
  Завершённый ответ содержит `status:"done"`, `verdict`, `time_ms`, фактический
  `output` и, при необходимости, `output_truncated` или `compile_output`.
  Для своего ввода успешное выполнение имеет вердикт `completed`.
  При сбое `status:"error"` сопровождается сообщением `error`.

Вывод и диагностика компиляции ограничены 16 КиБ каждый. Пробный запуск
занимает общий с оцениваемыми решениями слот исполнения на срок до минуты
плюс очистка контейнера. При занятом исполнителе или повторе раньше пяти секунд
сервер возвращает HTTP 429. Результат хранится в памяти до пяти минут
с общим пределом в 64 записи. Пробные запуски существуют отдельно от истории посылок.

## Проверка эталона

`POST /api/problems/{id}/reference-check` принимает
`{actor_id,language,source,test_revision?}` из аккаунта преподавателя.
Исполнитель компилирует код один раз и проверяет все примеры и скрытые тесты
выбранной версии. Если `test_revision` опущен, используется версия 1.
При ошибке компиляции запуск тестов невозможен. Общий срок
исполнения ограничен 90 секундами плюс очистка контейнера.

Ответ HTTP 202 и чтение результата работают через тот же временный кэш.
В отчёте присутствуют вердикт, время, количество пройденных и всех тестов,
номер первого непройденного теста и диагностика компиляции при её наличии.
Ввод, ожидаемый ответ и фактический вывод отдельных тестов остаются внутри
исполнителя. Эталон и отчёт существуют отдельно от истории посылок.
