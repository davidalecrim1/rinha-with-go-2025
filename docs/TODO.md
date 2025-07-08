# TODO

- [x] Criar uma versão inicial que atende os rascunhos dos endpoints.
- [x] Criar uma versão assincrona usando apenas channels e goroutines.
    - [x] Entender melhor o problema de retry que tem ocorrido quando rodo o K6.
    - [x] Entender melhor o tamanho estranho dos pools.
- [ ] Remover os logs da versão final.

## Parking Lot
- [ ] Refletir sobre a estratégia de endpoint default vs fallback e os custos do processamento.
    - Considerarei um Redis in memory compartilhado para determinar quando fazer fallback.
- [ ] Criar um job que inspeciona o health check de cada endpoint e atualiza algum status para tomada de decisão.
- [ ] Dado minha decisão de roteamento. Talvez o resty não atenda minhas necessidades. Pensar sobre.
- [ ] Considerar usar uma lib do sonic ou json-patch para ser mais rápido no encoding e decoding do body.
