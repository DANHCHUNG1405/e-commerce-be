INSERT INTO roles(name) VALUES ('driver') ON CONFLICT (name) DO NOTHING;
ALTER TABLE sellers ADD COLUMN description text NOT NULL DEFAULT '';
ALTER TABLE sellers ADD COLUMN pickup_address jsonb DEFAULT '{}';
CREATE TABLE driver_profiles (
 user_id uuid PRIMARY KEY REFERENCES users(id), phone text NOT NULL,
 vehicle_plate text NOT NULL, status text NOT NULL DEFAULT 'pending'
 CHECK (status IN ('pending','approved','rejected','suspended')),
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE shipments ADD COLUMN driver_id uuid REFERENCES driver_profiles(user_id);
ALTER TABLE shipments ADD COLUMN address_snapshot jsonb DEFAULT '{}';
ALTER TABLE shipments ADD COLUMN pickup_snapshot jsonb DEFAULT '{}';
ALTER TABLE shipments ADD COLUMN cod_amount bigint NOT NULL DEFAULT 0 CHECK (cod_amount >= 0);
ALTER TABLE shipments ADD COLUMN cod_collected boolean NOT NULL DEFAULT false;
ALTER TABLE shipments ADD COLUMN cod_settled boolean NOT NULL DEFAULT false;
ALTER TABLE shipments ADD CONSTRAINT shipment_cod_settlement CHECK (NOT cod_settled OR cod_collected);
ALTER TABLE shipments ADD CONSTRAINT internal_shipment_state CHECK (
 carrier <> 'internal' OR (status IN ('pending','assigned','accepted','picked_up','delivering','failed','returned','delivered')
 AND (status='pending' OR driver_id IS NOT NULL))
);
ALTER TABLE shipment_events ADD COLUMN actor_id uuid REFERENCES users(id);
ALTER TABLE shipment_events ADD COLUMN request_id text;
CREATE UNIQUE INDEX shipment_event_request ON shipment_events(shipment_id,request_id) WHERE request_id IS NOT NULL;
CREATE INDEX shipment_driver_status ON shipments(driver_id,status);
CREATE INDEX driver_status ON driver_profiles(status,user_id);
