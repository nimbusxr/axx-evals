Feature: Changing and cancelling a parcel, until a depot picks it up
  A shop can change or cancel a parcel while it is registered. Once a depot has picked it up,
  a change or a cancellation is refused, and the parcel stays as it was.

  Background:
    Given the parcels service with the following properties:
      | url     | http://localhost:8080              |
      | openapi | http://localhost:8080/openapi.json |
    And a parcels-db database with the following properties:
      | url      | jdbc:postgresql://postgres:5432/parcels |
      | user     | parcels                                 |
      | password | parcels                                 |

  Scenario: A picked-up parcel can no longer be changed, and keeps its weight
    Given a seeds/rules-picked-up-change.yaml db seed
    And a PATCH request to /api/parcels/PX-RULES-CHANGE-001
    And the request headers are:
      | Content-Type | application/json |
    And a request payload using an application/json empty content template
    And the request payload property weightGrams is '2400'
    And a 2nd ordered GET request to /api/parcels/PX-RULES-CHANGE-001
    When the request is executed
    And the 2nd ordered request is executed
    Then the response status code is 409
    And the response body contains 'can no longer be changed'
    And the 2nd ordered response status code is 200
    And the response payload property weightGrams is '1200' for 2nd ordered response

  Scenario: A picked-up parcel can no longer be cancelled, and can still be looked up
    Given a seeds/rules-picked-up-cancel.yaml db seed
    And a DELETE request to /api/parcels/PX-RULES-CANCEL-001
    And a 2nd ordered GET request to /api/parcels/PX-RULES-CANCEL-001
    When the request is executed
    And the 2nd ordered request is executed
    Then the response status code is 409
    And the response body contains 'can no longer be cancelled'
    And the 2nd ordered response status code is 200

  Scenario: A registered parcel can still be changed, and the change is stored
    Given a seeds/rules-registered.yaml db seed
    And a PATCH request to /api/parcels/PX-RULES-REGISTERED-001
    And the request headers are:
      | Content-Type | application/json |
    And a request payload using an application/json empty content template
    And the request payload property weightGrams is '2600'
    And a 2nd ordered GET request to /api/parcels/PX-RULES-REGISTERED-001
    When the request is executed
    And the 2nd ordered request is executed
    Then the response status code is 200
    And the 2nd ordered response status code is 200
    And the response payload property weightGrams is '2600' for 2nd ordered response
