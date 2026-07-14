import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import D1Stand from './D1Stand';
import D2ATISDialog from './D2ATISDialog';
import D2CDMDialog from './D2CDMDialog';
import D2PDCDialog from './D2PDCDialog';

describe('EFB operational dialogs', () => {
  it('does not claim unknown stand availability and keeps a rejected request open', async () => {
    const onClose = vi.fn();
    const onRequest = vi.fn().mockRejectedValue(new Error('stand is occupied'));
    render(<D1Stand isOpen onClose={onClose} stand="A12" onRequest={onRequest} />);

    expect(screen.getByTitle('A18 (availability checked when requested)')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Stand A18' }));
    fireEvent.click(screen.getByRole('button', { name: 'REQUEST NEW STAND' }));

    expect(await screen.findByRole('alert')).toHaveTextContent('stand is occupied');
    expect(onClose).not.toHaveBeenCalled();
  });

  it('shows ATIS as read-only current information', () => {
    render(<D2ATISDialog isOpen onClose={vi.fn()} position="L3" content="ATIS for SAS123" atisText="INFORMATION D" />);

    expect(screen.getByText('INFORMATION D')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'CLOSE' })).toBeInTheDocument();
    expect(screen.queryByText('ACKNOWLEDGE')).not.toBeInTheDocument();
  });

  it('keeps PDC open and displays a failed confirmation', async () => {
    const onClose = vi.fn();
    render(
      <D2PDCDialog
        isOpen
        onClose={onClose}
        onConfirm={vi.fn().mockRejectedValue(new Error('clearance changed'))}
        onUnable={vi.fn().mockResolvedValue(undefined)}
        position="L3"
        content="PDC for SAS123"
        pdcText="CLEARED AS FILED"
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: 'CONFIRM' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('clearance changed');
    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getByRole('button', { name: 'UNABLE' })).toBeInTheDocument();
  });

  it('rejects invalid TOBT locally and reports server rejection without closing', async () => {
    const onClose = vi.fn();
    const onUpdate = vi.fn().mockRejectedValue(new Error('CDM unavailable'));
    render(<D2CDMDialog isOpen onClose={onClose} currentTobt="1425Z" currentCtot="NIL" onUpdate={onUpdate} />);

    const input = screen.getByRole('textbox');
    fireEvent.change(input, { target: { value: '9999Z' } });
    fireEvent.click(screen.getByRole('button', { name: 'UPDATE TOBT' }));
    expect(screen.getByRole('alert')).toHaveTextContent('valid UTC time');
    expect(onUpdate).not.toHaveBeenCalled();

    fireEvent.change(input, { target: { value: '1430Z' } });
    fireEvent.click(screen.getByRole('button', { name: 'UPDATE TOBT' }));
    await waitFor(() => expect(onUpdate).toHaveBeenCalledWith('1430Z'));
    expect(await screen.findByRole('alert')).toHaveTextContent('CDM unavailable');
    expect(onClose).not.toHaveBeenCalled();
  });
});
