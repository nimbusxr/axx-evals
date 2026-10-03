Feature: Registering a parcel end to end
  A registration is checked with the address service, stored with the delivery zone it gives,
  and announced to billing and the depots as a ParcelRegistered event.

  Background:
    Given the parcels service with the following properties:
      | url     | http://localhost:8080              |
      | openapi | http://localhost:8080/openapi.json |
    And the mocked address-service service with the following properties:
      | url | http://address-service:8080 |
    And the parcels-kafka kafka service with the following properties:
      | brokers | kafka:9092 |
    And a parcel-events kafka topic client with the following properties:
      | consumer.value.deserializer  | io.confluent.kafka.serializers.KafkaAvroDeserializer |
      | consumer.schema.registry.url | http://schema-registry:8081                          |
      | consumer.auto.offset.reset   | earliest                                             |

  Scenario: A registered parcel is stored with the zone the address service gives
    Given a POST request to /api/parcels
    And the request headers are:
      | Content-Type | application/json |
    And a request payload using an application/json content example
    And the request payload properties are:
      | reference          | PX-E2E-STORED-001 |
      | sender             | shop-e2e-stored   |
      | recipient.city     | Lyon              |
      | recipient.postcode | 69002             |
      | recipient.country  | FR                |
    And a 2nd ordered GET request to /api/parcels/PX-E2E-STORED-001
    When the request is executed
    And the 2nd ordered request is executed
    Then the response status code is 201
    And the 2nd ordered response status code is 200
    And the response payload property zone is 'FR-1' for 2nd ordered response

  Scenario: Registering a parcel checks the postcode once, with our API key
    Given a POST request to /api/parcels
    And the request headers are:
      | Content-Type | application/json |
    And a request payload using an application/json content example
    And the request payload properties are:
      | reference          | PX-E2E-CHECK-001 |
      | sender             | shop-e2e-check   |
      | recipient.postcode | 10318            |
      | recipient.country  | DE               |
    When the request is executed
    Then the response status code is 201
    And the mocked GET request to /v1/postcodes/DE/10318 named postcode-10318 was received by address-service
    And the mocked request named postcode-10318 was received exactly 1 time
    And the header X-Api-Key for mocked request named postcode-10318 is 'evals-address-key'

  Scenario: An undeliverable postcode is refused and nothing is stored
    Given a POST request to /api/parcels
    And the request headers are:
      | Content-Type | application/json |
    And a request payload using an application/json content example
    And the request payload properties are:
      | reference          | PX-E2E-NODELIVERY-001 |
      | sender             | shop-e2e-nodelivery   |
      | recipient.postcode | 99930                 |
      | recipient.country  | DE                    |
    And a 2nd ordered GET request to /api/parcels/PX-E2E-NODELIVERY-001
    When the request is executed
    And the 2nd ordered request is executed
    Then the response status code is 422
    And the response body contains 'address not deliverable'
    And the 2nd ordered response status code is 404

  Scenario: A registered parcel is announced as a ParcelRegistered event
    Given a POST request to /api/parcels
    And the request headers are:
      | Content-Type | application/json |
    And a request payload using an application/json content example
    And the request payload properties are:
      | reference          | PX-E2E-EVENT-001 |
      | sender             | shop-e2e-event   |
      | weightGrams        | 3150             |
      | recipient.postcode | 04109            |
      | recipient.country  | DE               |
    When the request is executed
    Then the response status code is 201
    And the parcel-events kafka event named registered key is PX-E2E-EVENT-001
    And the parcel-events kafka event named registered payload properties are:
      | $.reference   | PX-E2E-EVENT-001 |
      | $.weightGrams | 3150             |
      | $.zone        | DE-1             |
