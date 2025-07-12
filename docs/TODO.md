# TODO

- [x] Criar uma versão inicial que atende os rascunhos dos endpoints.
- [x] Criar uma versão assincrona usando apenas channels e goroutines.
    - [x] Entender melhor o problema de retry que tem ocorrido quando rodo o K6.
    - [x] Entender melhor o tamanho estranho dos pools.
- [x] Criar uma versão para rodar com docker e nginx.
- [x] Remover o Resty e colocar minha própria logica de retry dado os problemas com o UpdateRequestedAt e testar.
- [x] Remover os logs da versão final.
- [x] Otimizar usando Sonic e net/http para cortar o máximo possivel de ms e evitar inconsistencias.
- [x] Considerar um sync.Pool para ajudar na performance.
- [x] Usar um channel para retry mais longo de forma async.
- [x] Entender o que irá me ajudar a resetar a inconsistencia dado que ainda acontece.
- [x] Usar um health check da API com Redis (last option).
- [x] Revisitar o problema de inconsistencias.
- [x] Fazer profilling para otimizar o channel e as gouroutines.
- [ ] Adicionar um banco de dados dado minha confusão com os endpoints admin.
- [ ] Entender se usarei Redis ou Mongo.
- [ ] Adicionar graceful shutdown para os workers.
