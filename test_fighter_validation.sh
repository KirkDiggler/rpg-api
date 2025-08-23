#!/bin/bash

echo "Testing Fighter validation..."

# Create a draft
DRAFT_ID=$(grpcurl -plaintext -d '{"player_id": "test-player"}' localhost:50051 dnd5e.api.v1alpha1.CharacterService/CreateDraft 2>/dev/null | grep draft_id | cut -d'"' -f4)
echo "Created draft: $DRAFT_ID"

# Update to Fighter class WITHOUT fighting style
echo -e "\n1. Testing Fighter without fighting style (should get validation warning):"
grpcurl -plaintext -d "{
  \"draft_id\": \"$DRAFT_ID\",
  \"class\": 5,
  \"class_choices\": []
}" localhost:50051 dnd5e.api.v1alpha1.CharacterService/UpdateClass 2>/dev/null | python3 -m json.tool | grep -A5 warnings

# Update to Fighter WITH fighting style
echo -e "\n2. Testing Fighter with fighting style:"
grpcurl -plaintext -d "{
  \"draft_id\": \"$DRAFT_ID\",
  \"class\": 5,
  \"class_choices\": [
    {
      \"category\": 11,
      \"source\": 4,
      \"choice_id\": \"fighter_feature_fighting-style-choice\",
      \"selection\": {
        \"fighting_style\": \"dueling\"
      }
    }
  ]
}" localhost:50051 dnd5e.api.v1alpha1.CharacterService/UpdateClass 2>/dev/null | python3 -m json.tool | grep -A5 warnings

echo -e "\nValidation test complete!"