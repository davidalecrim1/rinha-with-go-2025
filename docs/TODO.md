# TODO

- [x] Criar uma versão inicial que atende os rascunhos dos endpoints.
- [x] Criar uma versão assincrona usando apenas channels e goroutines.
    - [x] Entender melhor o problema de retry que tem ocorrido quando rodo o K6.
    - [x] Entender melhor o tamanho estranho dos pools.
- [x] Criar uma versão para rodar com docker e nginx.
- [x] Remover o Resty e colocar minha própria logica de retry dado os problemas com o UpdateRequestedAt e testar.
- [x] Remover os logs da versão final.
- [x] Otimizar usando Sonic e net/http para cortar o máximo possivel de ms e evitar inconsistencias.
- [ ] Considerrar um sync.Pool para ajudar na performance.
- [ ] Usar um channel para retry mais longo de forma async.
- [ ] Entender o que irá me ajudar a resetar a inconsistencia dado que ainda acontece.
- [ ] Usar um health check da API com Redis (last option).


## Parking Lot
- [ ] Refletir sobre a estratégia de endpoint default vs fallback e os custos do processamento.
    - Considerarei um Redis in memory compartilhado para determinar quando fazer fallback.
- [ ] Criar um job que inspeciona o health check de cada endpoint e atualiza algum status para tomada de decisão.
- [ ] Dado minha decisão de roteamento. Talvez o resty não atenda minhas necessidades. Pensar sobre.
- [ ] Considerar usar uma lib do sonic ou json-patch para ser mais rápido no encoding e decoding do body.
